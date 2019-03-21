package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	hornbillHelpers "github.com/hornbill/goHornbillHelpers"
)

//loadConfig -- Function to Load Configruation File
func loadConfig() (importConfStruct, bool) {
	boolLoadConf := true
	//-- Check Config File File Exists
	cwd, _ := os.Getwd()
	configurationFilePath := cwd + "/" + configFileName
	logger(1, "Loading Config File: "+configurationFilePath, false)
	if _, fileCheckErr := os.Stat(configurationFilePath); os.IsNotExist(fileCheckErr) {
		logger(4, "No Configuration File", true)
		os.Exit(102)
	}
	//-- Load Config File
	file, fileError := os.Open(configurationFilePath)
	//-- Check For Error Reading File
	if fileError != nil {
		logger(4, "Error Opening Configuration File: "+fmt.Sprintf("%v", fileError), true)
		boolLoadConf = false
	}

	//-- New Decoder
	decoder := json.NewDecoder(file)
	//-- New Var based on importConfStruct
	edbConf := importConfStruct{}
	//-- Decode JSON
	err := decoder.Decode(&edbConf)
	//-- Error Checking
	if err != nil {
		logger(4, "Error Decoding Configuration File: "+fmt.Sprintf("%v", err), true)
		boolLoadConf = false
	}
	//-- Return New Config
	return edbConf, boolLoadConf
}

//convExtendedColName - takes old extended column name, returns new one (supply h_custom_a returns h_custom_1 for example)
//Split string in to array with _ as seperator
//Convert last array entry string character to Rune
//Convert Rune to Integer
//Subtract 96 from Integer
//Convert resulting Integer to String (numeric character), append to prefix and pass back
func convExtendedColName(oldColName string) string {
	arrColName := strings.Split(oldColName, "_")
	strNewColID := strconv.Itoa(int([]rune(arrColName[2])[0]) - 96)
	return "h_custom_" + strNewColID
}

//parseFlags - grabs and parses command line flags
func parseFlags() {
	flag.StringVar(&configFileName, "file", "conf.json", "Name of the configuration file to load")
	flag.BoolVar(&configDryRun, "dryrun", false, "Dump import XML to log instead of creating requests")
	flag.BoolVar(&configVersion, "version", false, "Outputs version number and exits tool")
	flag.Parse()
}

//getRequestPrefix - gets and returns Service Manager request prefix
func getRequestPrefix(callclass string) string {
	espXmlmc := NewEspXmlmcSession(importConf.HBConf.APIKeys[0])

	strSetting := ""
	callclass = strings.ToLower(callclass)
	switch callclass {
	case "incident":
		strSetting = "guest.app.requests.types.IN"
	case "service request":
		strSetting = "guest.app.requests.types.SR"
	case "change request":
		strSetting = "app.requests.types.CH"
	case "problem":
		strSetting = "app.requests.types.PM"
	case "known error":
		strSetting = "app.requests.types.KE"
	case "release":
		strSetting = "app.requests.types.RM"
	}

	espXmlmc.SetParam("appName", appServiceManager)
	espXmlmc.SetParam("filter", strSetting)
	response, err := espXmlmc.Invoke("admin", "appOptionGet")
	if err != nil {
		logger(4, "Could not retrieve System Setting for Request Prefix. Using default ["+callclass+"].", false)
		return callclass
	}
	var xmlRespon xmlmcSysSettingResponse
	err = xml.Unmarshal([]byte(response), &xmlRespon)
	if err != nil {
		logger(4, "Could not retrieve System Setting for Request Prefix. Using default ["+callclass+"].", false)
		return callclass
	}
	if xmlRespon.MethodResult != "ok" {
		logger(4, "Could not retrieve System Setting for Request Prefix: "+xmlRespon.MethodResult, false)
		return callclass
	}
	return xmlRespon.Setting
}

func logger(t int, s string, outputToCLI bool) {
	hornbillHelpers.Logger(t, s, outputToCLI, localLogFileName)
}

func parseDateTime(dateTime, attribName string, buffer *bytes.Buffer) string {
	if importConf.DateTimeFormat != "" && dateTime != "" {
		t, err := time.Parse(importConf.DateTimeFormat, dateTime)
		if err != nil {
			buffer.WriteString(loggerGen(4, "parseDateTime Failed for ["+attribName+"]: "+fmt.Sprintf("%v", err)))
			buffer.WriteString(loggerGen(1, "[DATETIME] Failed to parse DateTime stamp "+dateTime+" against configuration DateTimeFormat "+importConf.DateTimeFormat))
		}
		return t.Format("2006-01-02 15:04:05")
	}
	return ""
}

func loggerGen(t int, s string) string {

	var errorLogPrefix = ""
	//-- Create Log Entry
	switch t {
	case 1:
		errorLogPrefix = "[DEBUG] "
	case 2:
		errorLogPrefix = "[MESSAGE] "
	case 3:
		errorLogPrefix = ""
	case 4:
		errorLogPrefix = "[ERROR] "
	case 5:
		errorLogPrefix = "[WARNING] "
	case 6:
		errorLogPrefix = ""
	}
	return errorLogPrefix + s + "\n\r"
}
func loggerWriteBuffer(s string) {
	if s != "" {
		logLines := strings.Split(s, "\n\r")
		for _, line := range logLines {
			if line != "" {
				logger(0, line, false)
			}
		}
	}
}
