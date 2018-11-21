package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/jmoiron/sqlx"
)

//buildConnectionString -- Build the connection string for the SQL driver
func buildConnectionString() string {
	connectString := ""

	//Build
	switch appDBDriver {
	case "mssql":
		connectString = "server=" + importConf.AppDBConf.Server
		connectString = connectString + ";database=" + importConf.AppDBConf.Database
		connectString = connectString + ";user id=" + importConf.AppDBConf.UserName
		connectString = connectString + ";password=" + importConf.AppDBConf.Password
		if !importConf.AppDBConf.Encrypt {
			connectString = connectString + ";encrypt=disable"
		}
		if importConf.AppDBConf.Port != 0 {
			dbPortSetting := strconv.Itoa(importConf.AppDBConf.Port)
			connectString = connectString + ";port=" + dbPortSetting
		}
	case "mysql":
		connectString = importConf.AppDBConf.UserName + ":" + importConf.AppDBConf.Password
		connectString = connectString + "@tcp(" + importConf.AppDBConf.Server + ":"
		if importConf.AppDBConf.Port != 0 {
			dbPortSetting := strconv.Itoa(importConf.AppDBConf.Port)
			connectString = connectString + dbPortSetting
		} else {
			connectString = connectString + "3306"
		}
		connectString = connectString + ")/" + importConf.AppDBConf.Database

	case "mysql320":
		dbPortSetting := strconv.Itoa(importConf.AppDBConf.Port)
		connectString = "tcp:" + importConf.AppDBConf.Server + ":" + dbPortSetting
		connectString = connectString + "*" + importConf.AppDBConf.Database + "/" + importConf.AppDBConf.UserName + "/" + importConf.AppDBConf.Password
	case "csv":
		connectString = "DSN=" + importConf.AppDBConf.Database + ";Extended Properties='text;HDR=Yes;FMT=Delimited'"
		appDBDriver = "odbc"
	case "odbc":
		connectString = "DSN=" + importConf.AppDBConf.Database + ";"
		appDBDriver = "odbc"
	}

	return connectString
}

//queryDBCallDetails -- Query call data & set map of calls to add to Hornbill
func queryDBCallDetails(serviceManagerRequestType, appRequestType, connString string) ([]map[string]interface{}, bool) {
	var arrCallDetailsMaps []map[string]interface{}
	dbapp, dberr := sqlx.Open(appDBDriver, connStrAppDB)
	if dberr != nil {
		logger(4, "Could not open application DB connection: "+dberr.Error(), true)
		return nil, false
	}
	defer dbapp.Close()
	//Check connection is open
	err := dbapp.Ping()
	if err != nil {
		logger(4, "[DATABASE] [PING] Database Connection Error: "+fmt.Sprintf("%v", err), true)
		return nil, false
	}
	logger(3, "[DATABASE] Connection Successful", true)
	logger(3, "[DATABASE] Retrieving "+serviceManagerRequestType+"s, "+appRequestType+" from the third party application.", true)
	logger(3, "[DATABASE] Please Wait...", true)
	//build query
	sqlCallQuery = mapGenericConf.RequestQuery
	logger(3, "[DATABASE] Query to retrieve "+serviceManagerRequestType+": "+sqlCallQuery, false)

	//Run Query
	rows, err := dbapp.Queryx(sqlCallQuery)

	if err != nil {
		logger(4, " Database Query Error: "+fmt.Sprintf("%v", err), true)
		return nil, false
	}
	defer rows.Close()
	//Build map full of calls to import
	intCallCount := 0
	for rows.Next() {
		intCallCount++
		results := make(map[string]interface{})
		err = rows.MapScan(results)
		if err != nil {
			//something is wrong with this row just log then skip it
			logger(4, " Database Result error"+err.Error(), true)
			continue
		}
		//Stick marshalled data map in to parent slice
		arrCallDetailsMaps = append(arrCallDetailsMaps, results)
	}
	return arrCallDetailsMaps, true
}

// getFieldValue --Retrieve field value from mapping via SQL record map
func getFieldValue(v string, u *map[string]interface{}) string {
	fieldMap := v
	recordMap := *u
	//-- Match $variable from String
	re1, err := regexp.Compile(`\[(.*?)\]`)
	if err != nil {
		color.Red("[ERROR] %v", err)
	}

	result := re1.FindAllString(fieldMap, 100)
	valFieldMap := ""
	//-- Loop Matches
	for _, val := range result {
		valFieldMap = ""
		valFieldMap = strings.Replace(val, "[", "", 1)
		valFieldMap = strings.Replace(valFieldMap, "]", "", 1)

		if recordMap[valFieldMap] != nil {
			if valField, ok := recordMap[valFieldMap].(int64); ok {
				valFieldMap = strconv.FormatInt(valField, 10)
			} else if valField, ok := recordMap[valFieldMap].(float64); ok {
				valFieldMap = strconv.FormatFloat(valField, 'f', -1, 64)
			} else {
				valFieldMap = fmt.Sprintf("%+s", recordMap[valFieldMap])
			}

			if valFieldMap != "<nil>" {
				fieldMap = strings.Replace(fieldMap, val, valFieldMap, 1)
			}
		} else {
			fieldMap = strings.Replace(fieldMap, val, "", 1)
		}
	}
	return fieldMap
}
