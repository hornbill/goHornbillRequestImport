package main

import (
	"bytes"
	"encoding/xml"
	"fmt"

	"github.com/hornbill/goApiLib"
)

// espLogger -- Log to ESP
func espLogger(message, severity string) {
	espXmlmc.SetParam("fileName", espLogFileName)
	espXmlmc.SetParam("group", "general")
	espXmlmc.SetParam("severity", severity)
	espXmlmc.SetParam("message", message)
	espXmlmc.Invoke("system", "logMessage")
}

//doesAnalystExist takes an Analyst ID string and returns a true if one exists in the cache or on the Instance
func doesAnalystExist(analystID string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) bool {
	boolAnalystExists := false
	if analystID != "" {
		analystIsInCache, strReturn := recordInCache(analystID, "Analyst")
		//-- Check if we have cached the Analyst already
		if analystIsInCache && strReturn != "" {
			boolAnalystExists = true
		} else {
			//Get Analyst Info
			espXmlmc.SetParam("userId", analystID)

			XMLAnalystSearch, xmlmcErr := espXmlmc.Invoke("admin", "userGetInfo")
			if xmlmcErr != nil {
				buffer.WriteString(loggerGen(4, "API Call Failed: Search for Analyst ["+analystID+"]: "+fmt.Sprintf("%v", xmlmcErr)))
			}

			var xmlRespon xmlmcAnalystListResponse
			err := xml.Unmarshal([]byte(XMLAnalystSearch), &xmlRespon)
			if err != nil {
				buffer.WriteString(loggerGen(4, "Response Unmarshal Failed: Search for Analyst ["+analystID+"]: "+fmt.Sprintf("%v", err)))
				return false
			}
			if xmlRespon.MethodResult != "ok" {
				//Analyst most likely does not exist
				buffer.WriteString(loggerGen(4, "MethodResult not OK: Search for Analyst ["+analystID+"]: "+xmlRespon.State.ErrorRet))
				return false
			}
			//-- Check Response
			if xmlRespon.AnalystFullName != "" {
				boolAnalystExists = true
				//-- Add Analyst to Cache
				var newAnalystForCache analystListStruct
				newAnalystForCache.AnalystID = analystID
				newAnalystForCache.AnalystName = xmlRespon.AnalystFullName
				buffer.WriteString(loggerGen(1, "Analyst Cached ["+analystID+"]: "+xmlRespon.AnalystFullName))
				analystNamedMap := []analystListStruct{newAnalystForCache}
				mutexAnalysts.Lock()
				analysts = append(analysts, analystNamedMap...)
				mutexAnalysts.Unlock()
			}
		}
	}
	return boolAnalystExists
}

//doesCustomerExist takes a Customer ID string and returns a true if one exists in the cache or on the Instance
func doesCustomerExist(customerID string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) bool {
	boolCustomerExists := false

	if customerID != "" {
		customerIsInCache, strReturn := recordInCache(customerID, "Customer")
		//-- Check if we have cached the Analyst already
		if customerIsInCache && strReturn != "" {
			boolCustomerExists = true
		} else {
			//Get Analyst Info
			espXmlmc.SetParam("customerId", customerID)
			espXmlmc.SetParam("customerType", importConf.CustomerType)
			XMLCustomerSearch, xmlmcErr := espXmlmc.Invoke("apps/"+appServiceManager, "shrGetCustomerDetails")
			if xmlmcErr != nil {
				buffer.WriteString(loggerGen(4, "API Call Failed: Search for Customer ["+customerID+"]: "+fmt.Sprintf("%v", xmlmcErr)))
				return false
			}

			var xmlRespon xmlmcCustomerListResponse
			err := xml.Unmarshal([]byte(XMLCustomerSearch), &xmlRespon)
			if err != nil {
				buffer.WriteString(loggerGen(4, "Response Unmarshal Failed: Search for Customer ["+customerID+"]: "+fmt.Sprintf("%v", err)))
				return false
			}
			if xmlRespon.MethodResult != "ok" {
				//Customer most likely does not exist
				buffer.WriteString(loggerGen(4, "MethodResult Not OK: Search for Customer ["+customerID+"]: "+xmlRespon.State.ErrorRet))
				return false
			}
			//-- Check Response
			if xmlRespon.CustomerFirstName != "" {
				boolCustomerExists = true
				//-- Add Customer to Cache
				var newCustomerForCache customerListStruct
				newCustomerForCache.CustomerID = customerID
				newCustomerForCache.CustomerName = xmlRespon.CustomerFirstName + " " + xmlRespon.CustomerLastName
				buffer.WriteString(loggerGen(1, "Customer Cached ["+customerID+"]: "+newCustomerForCache.CustomerName))
				customerNamedMap := []customerListStruct{newCustomerForCache}
				mutexCustomers.Lock()
				customers = append(customers, customerNamedMap...)
				mutexCustomers.Unlock()
			}

		}
	}
	return boolCustomerExists
}

//NewEspXmlmcSession - New Xmlmc Session variable (Cloned Session)
func NewEspXmlmcSession(apiKey string) *apiLib.XmlmcInstStruct {
	espXmlmcLocal := apiLib.NewXmlmcInstance(importConf.HBConf.InstanceID)
	espXmlmcLocal.SetAPIKey(apiKey)
	return espXmlmcLocal
}
