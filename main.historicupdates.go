package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	apiLib "github.com/hornbill/goApiLib"
	"github.com/hornbill/sqlx"
)

//getHistoricUpdates - query a list of historic updates for an imported request
func getHistoricUpdates(request *RequestDetails, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) {
	oldCallGUID := request.AppRequestGUID
	oldCallRef := request.AppRequestRef
	buffer.WriteString(loggerGen(1, "Retrieving Historic Updates for "+oldCallRef))
	sqlQuery := strings.Replace(request.GenericImportConf.RequestHistoricUpdateQuery, "{RequestGUID}", oldCallGUID, -1)
	buffer.WriteString(loggerGen(1, sqlQuery))
	dbapp, dberr := sqlx.Open(appDBDriver, connStrAppDB)
	if dberr != nil {
		buffer.WriteString(loggerGen(4, "Historic Updates: Could not open application DB connection: "+dberr.Error()))
		return
	}
	defer dbapp.Close()

	err := dbapp.Ping()
	if err != nil {
		buffer.WriteString(loggerGen(4, "[PING] Database Connection Error for Historical Updates: "+fmt.Sprintf("%v", err)))
		return
	}

	//Run Query
	rows, err := dbapp.Queryx(sqlQuery)

	if err != nil {
		buffer.WriteString(loggerGen(4, "Historic Update Database Query Error: "+fmt.Sprintf("%v", err)))
		buffer.WriteString(loggerGen(3, "Historic Update Query: "+sqlQuery))
		return
	}
	defer rows.Close()

	//Process each call diary entry, insert in to Hornbill
	for rows.Next() {
		diaryEntry := make(map[string]interface{})
		err = rows.MapScan(diaryEntry)
		if err != nil {
			buffer.WriteString(loggerGen(4, "Unable to retrieve data from SQL query: "+fmt.Sprintf("%v", err)))
			buffer.WriteString(loggerGen(3, "Historic Update Query: "+sqlQuery))
		} else {
			applyHistoricUpdate(diaryEntry, request, espXmlmc, buffer)
		}
	}
}

//applyHistoricalUpdate - takes an update from third party system, imports to Hornbill as Historical Update
func applyHistoricUpdate(diaryEntry map[string]interface{}, request *RequestDetails, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) {
	//Build slice to hold core columns
	coreFields := make(map[string]string)
	for k, v := range mapGenericConf.HistoricUpdateMapping {
		strAttribute := k
		strMapping := fmt.Sprint(v)

		if strAttribute == "h_updateby" && strMapping != "" {
			//Updater Analyst Name
			strOwnerID := getFieldValue(strMapping, &diaryEntry)
			if strOwnerID != "" {
				boolAnalystExists := doesAnalystExist(strOwnerID, espXmlmc, buffer)
				if boolAnalystExists {
					//Get analyst from cache as exists
					analystIsInCache, strOwnerName, _ := recordInCache(strOwnerID, "Analyst")
					if analystIsInCache && strOwnerName != "" {
						coreFields[strAttribute] = strOwnerID
						coreFields["h_updatebyname"] = strOwnerName
					}
				} else {
					coreFields[strAttribute] = strOwnerID
				}
			}
		} else if strAttribute == "h_updatebygroup" && strMapping != "" {
			//-- Get Team ID
			swTeamID := getFieldValue(strMapping, &diaryEntry)
			strTeamID, strTeamName := getCallTeamID(swTeamID, espXmlmc, buffer)
			if strTeamID == "" && mapGenericConf.DefaultTeam != "" {
				strTeamID = getTeamID(strTeamName, espXmlmc, buffer)
			}
			if strTeamID != "" && strTeamName != "" {
				coreFields[strAttribute] = strTeamID
			}
		} else if strAttribute == "h_updatedate" && strMapping != "" {
			strUpdateDate := parseDateTime(getFieldValue(strMapping, &diaryEntry), strAttribute, buffer)
			if strUpdateDate != "" {
				coreFields[strAttribute] = strUpdateDate
			}
		} else if strAttribute == "h_updatebyname" && strMapping != "" {
			value, ok := coreFields["h_updatebyname"]
			if !ok || value == "" {
				updateByName := getFieldValue(strMapping, &diaryEntry)
				if updateByName != "" {
					coreFields[strAttribute] = updateByName
				}
			}
		} else {
			coreFields[strAttribute] = getFieldValue(strMapping, &diaryEntry)
		}
	}

	espXmlmc.SetParam("application", appServiceManager)
	espXmlmc.SetParam("entity", "RequestHistoricUpdates")
	espXmlmc.SetParam("returnModifiedData", "true")
	espXmlmc.OpenElement("primaryEntityData")
	espXmlmc.OpenElement("record")
	espXmlmc.SetParam("h_fk_reference", request.SMRequestRef)
	for k, v := range coreFields {
		espXmlmc.SetParam(k, v)
	}

	espXmlmc.CloseElement("record")
	espXmlmc.CloseElement("primaryEntityData")
	XMLSTRING := espXmlmc.GetParam()
	//-- Check for Dry Run
	if !configDryRun {
		XMLUpdate, xmlmcErr := espXmlmc.Invoke("data", "entityAddRecord")
		if xmlmcErr != nil {
			buffer.WriteString(loggerGen(4, "API Invoke Failed Unable to add Historical Call Diary Update: "+fmt.Sprintf("%v", xmlmcErr)))
			buffer.WriteString(loggerGen(1, "Request Historical Update XML "+XMLSTRING))
			mutexCounters.Lock()
			counters.historicSkipped++
			mutexCounters.Unlock()
			return
		}
		var xmlRespon xmlmcRequestResponseStruct
		errXMLMC := xml.Unmarshal([]byte(XMLUpdate), &xmlRespon)
		if errXMLMC != nil {
			buffer.WriteString(loggerGen(4, "Unable to read response from Hornbill instance:"+fmt.Sprintf("%v", errXMLMC)))
			buffer.WriteString(loggerGen(1, "Request Historical Update XML "+XMLSTRING))
			mutexCounters.Lock()
			counters.historicSkipped++
			mutexCounters.Unlock()
			return
		}
		if xmlRespon.MethodResult != "ok" {
			buffer.WriteString(loggerGen(4, "API Call Failed Unable to add Historical Call Diary Update: "+xmlRespon.State.ErrorRet))
			buffer.WriteString(loggerGen(1, "Request Historical Update XML "+XMLSTRING))
			mutexCounters.Lock()
			counters.historicSkipped++
			mutexCounters.Unlock()
			return
		}
		buffer.WriteString(loggerGen(1, "Historic Update Added ["+xmlRespon.HistoricUpdateID+"]"))
	} else {
		buffer.WriteString(loggerGen(1, "Request Historical Update XML "+XMLSTRING))
		mutexCounters.Lock()
		counters.historicSkipped++
		mutexCounters.Unlock()
		espXmlmc.ClearParam()
		return
	}
	mutexCounters.Lock()
	counters.historicUpdated++
	mutexCounters.Unlock()
}
