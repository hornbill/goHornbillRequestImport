package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"

	apiLib "github.com/hornbill/goApiLib"
	"github.com/hornbill/pb"
)

var (
	csvHistoricUpdatesCache = make(map[string][]map[string]interface{})
	mutexCSVHistoricUpdates = &sync.Mutex{}
)

type customReader struct{ r io.Reader }

func (r *customReader) Read(b []byte) (n int, err error) {
	x := make([]byte, len(b))
	if n, err = r.r.Read(x); err != nil {
		return n, err
	}
	copy(b, bytes.Replace(x, []byte("\r"), []byte("\n"), -1))
	return n, nil
}

// getCSVRecords - reads a CSV file, returns a slice of maps of the records within, keyed on column header
func getCSVRecords(csvFilePath string) ([]map[string]interface{}, error) {
	var arrRecordMaps []map[string]interface{}

	file, err := os.Open(csvFilePath)
	if err != nil {
		return arrRecordMaps, err
	}
	defer file.Close()

	//Detect and skip a UTF-8 BOM if present
	bom := make([]byte, 3)
	bytesRead, _ := file.Read(bom)
	if !(bytesRead == 3 && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF) {
		// No BOM Detected, reset the file feed
		file.Seek(0, 0)
	}

	var r *csv.Reader
	if importConf.CSVConf.CarriageReturnRemoval {
		custom := &customReader{file}
		r = csv.NewReader(custom)
	} else {
		r = csv.NewReader(file)
	}
	//because the json configuration loader cannot handle runes, code here to convert string to rune-array and getting first item
	if importConf.CSVConf.CommaCharacter != "" {
		csvCommaRunes := []rune(importConf.CSVConf.CommaCharacter)
		r.Comma = csvCommaRunes[0]
	}
	if importConf.CSVConf.LazyQuotes {
		r.LazyQuotes = true
	}
	if importConf.CSVConf.FieldsPerRecord > 0 {
		r.FieldsPerRecord = importConf.CSVConf.FieldsPerRecord
	}

	var header []string
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return arrRecordMaps, err
		}
		if header == nil {
			header = record
		} else {
			var dict = make(map[string]interface{})
			for i := range header {
				if i < len(record) {
					dict[header[i]] = record[i]
				}
			}
			arrRecordMaps = append(arrRecordMaps, dict)
		}
	}
	return arrRecordMaps, nil
}

// queryCSVCallDetails -- Read call data from CSV file & set map of calls to add to Hornbill
func queryCSVCallDetails(serviceManagerRequestType, appRequestType string) ([]map[string]interface{}, bool, int) {
	var arrCallDetailsMaps []map[string]interface{}
	if mapGenericConf.CSVFile == "" {
		logger(4, "[CSV] CSVFile not set in configuration for request type: "+serviceManagerRequestType+" ["+appRequestType+"]", true)
		return nil, false, 0
	}
	logger(3, "[CSV] Retrieving "+serviceManagerRequestType+"s, "+appRequestType+" from CSV file: "+mapGenericConf.CSVFile, true)
	logger(3, "[CSV] Please Wait...", true)

	records, err := getCSVRecords(mapGenericConf.CSVFile)
	if err != nil {
		logger(4, "[CSV] Error reading CSV file ["+mapGenericConf.CSVFile+"]: "+err.Error(), true)
		return nil, false, 0
	}

	intCallCount := 0
	for _, results := range records {
		counters.found++

		if results[mapGenericConf.RequestReferenceColumn] == nil {
			logger(4, "Record contains nil value in Reference Column, or Reference Column does not exist in CSV file: "+mapGenericConf.RequestReferenceColumn, false)
			counters.createdSkipped++
			continue
		}
		s := fmt.Sprintf("%v", results[mapGenericConf.RequestReferenceColumn])

		requestIsInCache, _, _ := recordInCache(s, "Request")
		if requestIsInCache {
			continue
		}
		mutexRequests.Lock()
		var r requestListStruct
		r.RequestID = s
		requests = append(requests, r)
		mutexRequests.Unlock()
		intCallCount++

		//Stick marshalled data map in to parent slice
		arrCallDetailsMaps = append(arrCallDetailsMaps, results)
	}
	return arrCallDetailsMaps, true, intCallCount
}

// processCallDataCSV - Read CSV call data, process accordingly
func processCallDataCSV() {
	arrCallDetailsMaps, success, returnedCalls := queryCSVCallDetails(mapGenericConf.ServiceManagerRequestType, mapGenericConf.AppRequestType)
	if success && returnedCalls == 0 {
		logger(4, "No Request records found for type: "+mapGenericConf.ServiceManagerRequestType+" ["+mapGenericConf.AppRequestType+"]", true)
		return
	}
	if success {
		bar := pb.StartNew(len(arrCallDetailsMaps))
		defer bar.FinishPrint(mapGenericConf.ServiceManagerRequestType + " Request Import Complete")

		jobs := make(chan RequestDetails, configMaxRoutines)

		for w := 0; w < configMaxRoutines; w++ {
			wg.Add(1)
			espXmlmc := NewEspXmlmcSession(importConf.HBConf.APIKeys[w])
			go logNewCallJobs(jobs, &wg, espXmlmc)
		}

		for _, callRecord := range arrCallDetailsMaps {
			mutexBar.Lock()
			bar.Increment()
			mutexBar.Unlock()
			jobs <- RequestDetails{GenericImportConf: mapGenericConf, CallMap: callRecord}
		}

		close(jobs)
		wg.Wait()
	} else {
		logger(4, "Request search failed for type: "+mapGenericConf.ServiceManagerRequestType+" ["+mapGenericConf.AppRequestType+"]", true)
	}
}

// getCachedCSVRecords - returns the records from the given CSV file, reading the file only on first access
func getCachedCSVRecords(csvFilePath string) ([]map[string]interface{}, error) {
	mutexCSVHistoricUpdates.Lock()
	defer mutexCSVHistoricUpdates.Unlock()
	if records, ok := csvHistoricUpdatesCache[csvFilePath]; ok {
		return records, nil
	}
	records, err := getCSVRecords(csvFilePath)
	if err != nil {
		return nil, err
	}
	csvHistoricUpdatesCache[csvFilePath] = records
	return records, nil
}

// getHistoricUpdatesCSV - retrieve historic updates for an imported request from a CSV file
func getHistoricUpdatesCSV(request *RequestDetails, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) {
	if request.GenericImportConf.CSVHistoricUpdatesFile == "" {
		return
	}
	if request.GenericImportConf.CSVHistoricUpdatesRefColumn == "" {
		buffer.WriteString(loggerGen(4, "Historic Updates: CSVHistoricUpdatesRefColumn not set in configuration"))
		return
	}
	//Match on the source request GUID, falling back to the source request reference
	matchValue := request.AppRequestGUID
	if matchValue == "" {
		matchValue = request.AppRequestRef
	}
	if matchValue == "" {
		buffer.WriteString(loggerGen(5, "Historic Updates: no source request GUID or reference to match against, skipping"))
		return
	}
	buffer.WriteString(loggerGen(1, "Retrieving Historic Updates for "+request.AppRequestRef+" from CSV file: "+request.GenericImportConf.CSVHistoricUpdatesFile))

	records, err := getCachedCSVRecords(request.GenericImportConf.CSVHistoricUpdatesFile)
	if err != nil {
		buffer.WriteString(loggerGen(4, "Historic Updates: Error reading CSV file ["+request.GenericImportConf.CSVHistoricUpdatesFile+"]: "+err.Error()))
		return
	}

	intUpdateCount := 0
	for _, diaryEntry := range records {
		if diaryEntry[request.GenericImportConf.CSVHistoricUpdatesRefColumn] == nil {
			continue
		}
		if fmt.Sprintf("%v", diaryEntry[request.GenericImportConf.CSVHistoricUpdatesRefColumn]) == matchValue {
			applyHistoricUpdate(diaryEntry, request, espXmlmc, buffer)
			intUpdateCount++
		}
	}
	buffer.WriteString(loggerGen(1, strconv.Itoa(intUpdateCount)+" Historic Updates found for "+request.AppRequestRef))
}
