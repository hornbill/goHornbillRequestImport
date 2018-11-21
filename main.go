package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"

	_ "github.com/alexbrainman/odbc"     //ODBC Driver
	_ "github.com/denisenkom/go-mssqldb" //Microsoft SQL Server driver - v2005+
	_ "github.com/go-sql-driver/mysql"   //MySQL v4.1 to v5.x and MariaDB driver
	_ "github.com/weave-lab/mysql320"    //MySQL v3.2.0 to v5 driver - Provides SWSQL (MySQL 4.0.16) support
)

// main package
func main() {
	//-- Start Time for Durration
	startTime = time.Now()
	//-- Start Time for Log File
	timeNow = time.Now().Format(time.RFC3339)
	timeNow = strings.Replace(timeNow, ":", "-", -1)

	localLogFileName += timeNow

	parseFlags()

	//-- Output to CLI and Log
	logger(1, "---- Hornbill Service Manager Request Import Utility V"+fmt.Sprintf("%v", version)+" ----", true)
	logger(1, "Flag - Config File "+configFileName, true)
	logger(1, "Flag - Dry Run "+fmt.Sprintf("%v", configDryRun), true)
	logger(1, "Flag - Concurrent Requests "+fmt.Sprintf("%v", configMaxRoutines), true)

	//-- Load Configuration File Into Struct
	importConf, boolConfLoaded = loadConfig()
	if !boolConfLoaded {
		logger(4, "Unable to load config, process closing.", true)
		return
	}

	configMaxRoutines = len(importConf.HBConf.APIKeys)
	if configMaxRoutines < 1 || configMaxRoutines > 10 {
		color.Red("The maximum allowed workers is between 1 and 10 (inclusive).\n\n")
		color.Red("You have included " + strconv.Itoa(configMaxRoutines) + " API keys. Please try again, with a valid number of keys.")
		return
	}

	//Set SQL driver ID string for Application Data
	if importConf.AppDBConf.Driver == "" {
		logger(4, "AppDBConf SQL Driver not set in configuration.", true)
		return
	}
	if importConf.AppDBConf.Driver == "swsql" {
		appDBDriver = "mysql320"
	} else if importConf.AppDBConf.Driver == "mysql" ||
		importConf.AppDBConf.Driver == "mssql" ||
		importConf.AppDBConf.Driver == "mysql320" ||
		importConf.AppDBConf.Driver == "odbc" ||
		importConf.AppDBConf.Driver == "csv" {
		appDBDriver = importConf.AppDBConf.Driver
	} else {
		logger(4, "The driver ("+importConf.AppDBConf.Driver+") for the Application Database specified in the configuration file is not valid.", true)
		return
	}

	//-- Build DB connection string
	connStrAppDB = buildConnectionString()

	//Get request type import config, process each in turn
	for _, val := range importConf.RequestTypesToImport {
		if val.Import {
			reqPrefix = getRequestPrefix(val.ServiceManagerRequestType)
			mapGenericConf = val
			if appDBDriver == "odbc" ||
				appDBDriver == "xls" ||
				appDBDriver == "csv" {
				processCallDataODBC()
			} else {
				processCallData()

			}
		}
	}
	if len(arrSpawnBPMs) > 0 {
		processBPMs()
	}

	//-- End output
	logger(3, "", true)
	logger(3, "Requests Logged: "+fmt.Sprintf("%d", counters.created), true)
	logger(3, "Requests Skipped: "+fmt.Sprintf("%d", counters.createdSkipped), true)
	logger(3, "Requests with available BPM Workflows: "+fmt.Sprintf("%d", counters.bpmAvailable), true)
	logger(3, "BPM Workflows Spawned: "+fmt.Sprintf("%d", counters.bpmSpawned), true)
	logger(3, "BPM Workflows Associated to Requests: "+fmt.Sprintf("%d", counters.bpmRequest), true)
	logger(3, "Historic Updates Created: "+fmt.Sprintf("%d", counters.historicUpdated), true)
	logger(3, "Historic Updates Skipped: "+fmt.Sprintf("%d", counters.historicSkipped), true)
	//-- Show Time Takens
	endTime = time.Since(startTime)
	logger(3, "Time Taken: "+fmt.Sprintf("%v", endTime), true)
	logger(3, "---- Hornbill Service Manager Request Import Complete ---- ", true)
}
