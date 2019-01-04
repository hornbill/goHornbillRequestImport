package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"

	"github.com/hornbill/goApiLib"
)

//doesAnalystExist takes an Analyst ID string and returns a true if one exists in the cache or on the Instance
func doesAnalystExist(analystID string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) bool {
	boolAnalystExists := false
	if analystID != "" {
		analystIsInCache, strReturn, _ := recordInCache(analystID, "Analyst")
		//-- Check if we have cached the Analyst already
		if analystIsInCache && strReturn != "" {
			boolAnalystExists = true
		} else {
			//Get Analyst Info
			espXmlmc.SetParam("userId", analystID)

			XMLAnalystSearch, xmlmcErr := espXmlmc.Invoke("admin", "userGetInfo")
			if xmlmcErr != nil {
				buffer.WriteString(loggerGen(4, "API Call Failed: Search for User ["+analystID+"]: "+fmt.Sprintf("%v", xmlmcErr)))
			}

			var xmlRespon xmlmcAnalystListResponse
			err := xml.Unmarshal([]byte(XMLAnalystSearch), &xmlRespon)
			if err != nil {
				buffer.WriteString(loggerGen(4, "Response Unmarshal Failed: Search for User ["+analystID+"]: "+fmt.Sprintf("%v", err)))
				return false
			}
			if xmlRespon.MethodResult != "ok" {
				//Analyst most likely does not exist
				buffer.WriteString(loggerGen(4, "Search for User ["+analystID+"]: "+xmlRespon.State.ErrorRet))
				return false
			}
			//-- Check Response
			if xmlRespon.AnalystFullName != "" {
				boolAnalystExists = true
				//-- Add Analyst to Cache
				var newAnalystForCache analystListStruct
				newAnalystForCache.AnalystID = analystID
				newAnalystForCache.AnalystName = xmlRespon.AnalystFullName
				buffer.WriteString(loggerGen(1, "User Cached ["+analystID+"]: "+xmlRespon.AnalystFullName))
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
func doesContactExist(customerID string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) bool {
	boolCustomerExists := false

	if customerID != "" {
		customerIsInCache, strReturn, intReturn := recordInCache(customerID, "Customer")
		//-- Check if we have cached the Analyst already
		if customerIsInCache && strReturn != "" && intReturn > 0 {
			boolCustomerExists = true
		} else {
			//Get Analyst Info
			espXmlmc.SetParam("entity", "Contact")
			espXmlmc.SetParam("matchScope", "all")
			espXmlmc.OpenElement("searchFilter")
			espXmlmc.SetParam("column", "h_contact_status")
			espXmlmc.SetParam("value", "0")
			espXmlmc.CloseElement("searchFilter")
			espXmlmc.OpenElement("searchFilter")
			espXmlmc.SetParam("column", importConf.ContactKeyColumn)
			espXmlmc.SetParam("value", customerID)
			espXmlmc.CloseElement("searchFilter")

			XMLCustomerSearch, xmlmcErr := espXmlmc.Invoke("data", "entityBrowseRecords2")
			if xmlmcErr != nil {
				buffer.WriteString(loggerGen(4, "API Call Failed: Search for Contact ["+customerID+"]: "+fmt.Sprintf("%v", xmlmcErr)))
				return false
			}

			var xmlRespon xmlmcCustomerListResponse
			err := xml.Unmarshal([]byte(XMLCustomerSearch), &xmlRespon)
			if err != nil {
				buffer.WriteString(loggerGen(4, "Response Unmarshal Failed: Search for Contact ["+customerID+"]: "+fmt.Sprintf("%v", err)))
				return false
			}
			if xmlRespon.MethodResult != "ok" {
				//Customer most likely does not exist
				buffer.WriteString(loggerGen(4, "Search for Contact ["+customerID+"]: "+xmlRespon.State.ErrorRet))
				return false
			}
			//-- Check Response
			if xmlRespon.CustomerFirstName != "" {
				boolCustomerExists = true
				//-- Add Customer to Cache
				var newCustomerForCache customerListStruct
				newCustomerForCache.CustomerLoginID = customerID
				newCustomerForCache.CustomerID = xmlRespon.CustomerID
				newCustomerForCache.CustomerName = xmlRespon.CustomerFirstName + " " + xmlRespon.CustomerLastName
				buffer.WriteString(loggerGen(1, "Contact Cached ["+customerID+"]: ["+strconv.Itoa(xmlRespon.CustomerID)+"] "+newCustomerForCache.CustomerName))
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
