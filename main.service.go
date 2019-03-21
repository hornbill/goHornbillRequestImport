package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	apiLib "github.com/hornbill/goApiLib"
)

//getCallServiceID takes the Call Record and returns a correct Service ID if one exists on the Instance
func getCallServiceID(swService string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) string {
	serviceID := ""
	serviceName := ""
	if importConf.ServiceMapping[swService] != nil {
		serviceName = fmt.Sprintf("%s", importConf.ServiceMapping[swService])

		if serviceName != "" {
			serviceID = getServiceID(serviceName, espXmlmc, buffer)
		}
	}
	return serviceID
}

//getServiceID takes a Service Name string and returns a correct Service ID if one exists in the cache or on the Instance
func getServiceID(serviceName string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) string {
	serviceID := ""
	if serviceName != "" {
		serviceIsInCache, ServiceIDCache, _ := recordInCache(serviceName, "Service")
		//-- Check if we have cached the Service already
		if serviceIsInCache {
			serviceID = ServiceIDCache
		} else {
			serviceIsOnInstance, ServiceIDInstance := searchService(serviceName, espXmlmc, buffer)
			//-- If Returned set output
			if serviceIsOnInstance {
				serviceID = strconv.Itoa(ServiceIDInstance)
			}
		}
	}
	return serviceID
}

// seachService -- Function to check if passed-through service name is on the instance
func searchService(serviceName string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) (bool, int) {
	boolReturn := false
	intReturn := 0
	//-- ESP Query for service
	espXmlmc.SetParam("application", appServiceManager)
	espXmlmc.SetParam("entity", "Services")
	espXmlmc.SetParam("matchScope", "all")
	espXmlmc.OpenElement("searchFilter")
	espXmlmc.SetParam("column", "h_servicename")
	espXmlmc.SetParam("value", serviceName)
	espXmlmc.SetParam("matchType", "exact")
	espXmlmc.CloseElement("searchFilter")
	espXmlmc.SetParam("maxResults", "1")

	XMLServiceSearch, xmlmcErr := espXmlmc.Invoke("data", "entityBrowseRecords2")
	if xmlmcErr != nil {
		buffer.WriteString(loggerGen(4, "API Call Failed: Search Service ["+serviceName+"]: "+fmt.Sprintf("%v", xmlmcErr)))
		return boolReturn, intReturn
	}
	var xmlRespon xmlmcServiceListResponse

	err := xml.Unmarshal([]byte(XMLServiceSearch), &xmlRespon)
	if err != nil {
		buffer.WriteString(loggerGen(4, "Response Unmarshal Failed: Search Service ["+serviceName+"]: "+fmt.Sprintf("%v", err)))
		return boolReturn, intReturn
	}
	if xmlRespon.MethodResult != "ok" {
		buffer.WriteString(loggerGen(5, "MethodResult Not OK: Search Service ["+serviceName+"]: "+xmlRespon.State.ErrorRet))
		return boolReturn, intReturn
	}
	//-- Check Response
	if xmlRespon.ServiceName != "" {
		if strings.ToLower(xmlRespon.ServiceName) == strings.ToLower(serviceName) {
			intReturn = xmlRespon.ServiceID
			boolReturn = true
			//-- Add Service to Cache
			var newServiceForCache serviceListStruct
			newServiceForCache.ServiceID = intReturn
			newServiceForCache.ServiceName = serviceName
			newServiceForCache.ServiceBPMIncident = xmlRespon.BPMIncident
			newServiceForCache.ServiceBPMService = xmlRespon.BPMService
			newServiceForCache.ServiceBPMChange = xmlRespon.BPMChange
			newServiceForCache.ServiceBPMProblem = xmlRespon.BPMProblem
			newServiceForCache.ServiceBPMKnownError = xmlRespon.BPMKnownError
			newServiceForCache.ServiceBPMRelease = xmlRespon.BPMRelease
			newServiceForCache.CatalogItems = getCatalogItems(intReturn, espXmlmc, buffer)
			serviceNamedMap := []serviceListStruct{newServiceForCache}
			mutexServices.Lock()
			services = append(services, serviceNamedMap...)
			mutexServices.Unlock()
			buffer.WriteString(loggerGen(1, "Service Cached ["+serviceName+"] ["+strconv.Itoa(xmlRespon.ServiceID)+"]"))
		}
	}

	//Return Service ID once cached - we can now use this in the calling function to get all details from cache
	return boolReturn, intReturn
}

func getCatalogItems(serviceID int, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) []catalogItemListStruct {
	//pagination support
	returnTotalRows := 100
	rowStart := 0

	//get first page & total count
	page, count := getPageCatalogItems(serviceID, returnTotalRows, rowStart, espXmlmc, buffer)
	if len(page) == count || count == 0 {
		return page
	}
	//Count bigger than page, go through the rest
	catalogItems := page
	x := len(catalogItems)

	for x < count {
		nextPage, totalItems := getPageCatalogItems(serviceID, returnTotalRows, x, espXmlmc, buffer)
		if totalItems == 0 {
			break
		}
		catalogItems = append(catalogItems, nextPage...)
		x = len(catalogItems)
	}
	return catalogItems
}

func getPageCatalogItems(serviceID, limit, rowStart int, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) ([]catalogItemListStruct, int) {
	var foundCatalogItems []catalogItemListStruct
	intReturn := 0
	espXmlmc.SetParam("application", "com.hornbill.servicemanager")
	espXmlmc.SetParam("queryName", "getCatalogs")
	espXmlmc.SetParam("returnFoundRowCount", "true")
	espXmlmc.OpenElement("queryParams")
	espXmlmc.SetParam("language", "default")
	espXmlmc.SetParam("serviceId", strconv.Itoa(serviceID))
	espXmlmc.SetParam("rowStart", strconv.Itoa(rowStart))
	espXmlmc.SetParam("limit", strconv.Itoa(limit))
	espXmlmc.CloseElement("queryParams")
	XMLServiceSearch, xmlmcErr := espXmlmc.Invoke("data", "queryExec")
	if xmlmcErr != nil {
		buffer.WriteString(loggerGen(4, "API Call Failed: Search Catalog Items for Service ID ["+strconv.Itoa(serviceID)+"]: "+fmt.Sprintf("%v", xmlmcErr)))
		return foundCatalogItems, intReturn
	}
	var xmlRespon xmlmcCatalogItemListResponse
	err := xml.Unmarshal([]byte(XMLServiceSearch), &xmlRespon)
	if err != nil {
		buffer.WriteString(loggerGen(4, "Response Unmarshal Failed: Search Catalog Items for Service ID ["+strconv.Itoa(serviceID)+"]: "+fmt.Sprintf("%v", err)))
		return foundCatalogItems, intReturn
	}
	if xmlRespon.MethodResult != "ok" {
		buffer.WriteString(loggerGen(5, "MethodResult Not OK: Search Catalog Items for Service ID ["+strconv.Itoa(serviceID)+"]: "+xmlRespon.State.ErrorRet))
		return foundCatalogItems, intReturn
	}
	//-- Check Response
	foundCatalogItems = xmlRespon.CatalogItems
	intReturn = xmlRespon.FoundRows
	return foundCatalogItems, intReturn
}
