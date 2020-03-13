package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"sync"

	apiLib "github.com/hornbill/goApiLib"
	"github.com/hornbill/pb"
)

//processCallData - Query External call data, process accordingly
func processCallData() {
	arrCallDetailsMaps, success, returnedCalls := queryDBCallDetails(mapGenericConf.ServiceManagerRequestType, mapGenericConf.AppRequestType, connStrAppDB)
	if success && returnedCalls > 0 {

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
		logger(4, "Request search failed for type: "+mapGenericConf.ServiceManagerRequestType+"["+mapGenericConf.AppRequestType+"]", true)
	}
}

//processCallDataODBC - Query ODBC call data, process accordingly
func processCallDataODBC() {
	arrCallDetailsMaps, success, returnedCalls := queryDBCallDetails(mapGenericConf.ServiceManagerRequestType, mapGenericConf.AppRequestType, connStrAppDB)
	if success && returnedCalls == 0 {
		logger(4, "No Request records found for type: "+mapGenericConf.ServiceManagerRequestType+" ["+mapGenericConf.AppRequestType+"]", true)
		return
	}
	if success {
		bar := pb.StartNew(len(arrCallDetailsMaps))
		defer bar.FinishPrint(mapGenericConf.ServiceManagerRequestType + " Request Import Complete")
		espXmlmc := NewEspXmlmcSession(importConf.HBConf.APIKeys[0])

		newCallRef := ""
		oldCallRef := ""
		oldCallGUID := ""
		for _, callRecord := range arrCallDetailsMaps {
			mutexBar.Lock()
			bar.Increment()
			mutexBar.Unlock()

			var buffer bytes.Buffer

			if callRecord[mapGenericConf.RequestReferenceColumn] != nil {
				buffer.WriteString(loggerGen(3, "   "))
				oldCallRef, newCallRef, oldCallGUID = logNewCall(RequestDetails{GenericImportConf: mapGenericConf, CallMap: callRecord}, espXmlmc, &buffer)
			}
			//Process Historic Updates
			getHistoricUpdates(&RequestDetails{GenericImportConf: mapGenericConf, CallMap: callRecord, AppRequestRef: oldCallRef, SMRequestRef: newCallRef, AppRequestGUID: oldCallGUID}, espXmlmc, &buffer)

			bufferMutex.Lock()
			loggerWriteBuffer(buffer.String())
			bufferMutex.Unlock()
			buffer.Reset()
		}
	} else {
		logger(4, "Request search failed for type: "+mapGenericConf.ServiceManagerRequestType+" ["+mapGenericConf.AppRequestType+"]", true)
	}
}

func processBPMs() {
	logger(3, " ", false)
	logger(1, "[BPM] Spawning workflows for "+strconv.Itoa(len(arrSpawnBPMs))+" Requests", true)
	bar := pb.StartNew(len(arrSpawnBPMs))
	defer bar.FinishPrint("BPM spawning complete")

	jobs := make(chan spawnBPMStruct, configMaxRoutines)

	for w := 0; w < configMaxRoutines; w++ {
		wg.Add(1)
		espXmlmc := NewEspXmlmcSession(importConf.HBConf.APIKeys[w])
		go spawnBPM(jobs, &wg, espXmlmc)
	}

	for _, requestRecord := range arrSpawnBPMs {
		mutexBar.Lock()
		bar.Increment()
		mutexBar.Unlock()
		jobs <- requestRecord
	}
	close(jobs)
	wg.Wait()
}

//logNewCallJobs - Function takes external call data in a map, and logs to Hornbill
func logNewCallJobs(jobs chan RequestDetails, wg *sync.WaitGroup, espXmlmc *apiLib.XmlmcInstStruct) {
	defer wg.Done()
	for request := range jobs {
		var buffer bytes.Buffer
		buffer.WriteString(loggerGen(3, "   "))

		request.AppRequestRef, request.SMRequestRef, request.AppRequestGUID = logNewCall(request, espXmlmc, &buffer)

		getHistoricUpdates(&request, espXmlmc, &buffer)

		bufferMutex.Lock()
		loggerWriteBuffer(buffer.String())
		bufferMutex.Unlock()
		buffer.Reset()
	}
}

func logNewCall(request RequestDetails, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) (string, string, string) {
	oldReference := ""
	oldGUID := ""
	newReference := ""

	oldRequestRefMapping := "[" + request.GenericImportConf.RequestReferenceColumn + "]"
	oldReference = getFieldValue(oldRequestRefMapping, &request.CallMap)

	oldGUIDMapping := "[" + request.GenericImportConf.RequestGUID + "]"
	oldGUID = getFieldValue(oldGUIDMapping, &request.CallMap)

	buffer.WriteString(loggerGen(1, "Source Request Reference & GUID: ["+oldReference+"] ["+oldGUID+"]"))

	strStatus := "status.open"
	boolOnHoldRequest := false

	//Get request status from request & map
	statusMapping := fmt.Sprintf("%v", mapGenericConf.CoreFieldMapping["h_status"])
	strStatusID := getFieldValue(statusMapping, &request.CallMap)
	if importConf.StatusMapping[strStatusID] != nil {
		strStatus = fmt.Sprintf("%v", importConf.StatusMapping[strStatusID])
	}

	//Build slice to hold core columns
	coreFields := make(map[string]string)
	strAttribute := ""
	strMapping := ""
	strServiceBPM := ""
	boolUpdateLogDate := false
	strLoggedDate := ""
	strClosedDate := ""

	//Loop through core fields from config, add to XMLMC Params
	for k, v := range mapGenericConf.CoreFieldMapping {
		boolAutoProcess := true
		strAttribute = fmt.Sprintf("%v", k)
		strMapping = fmt.Sprintf("%v", v)

		//Owning Analyst Name
		if strAttribute == "h_ownerid" {
			strOwnerID := getFieldValue(strMapping, &request.CallMap)
			strOwnerName := ""
			if strOwnerID != "" {
				boolAnalystExists := doesAnalystExist(strOwnerID, espXmlmc, buffer)
				if boolAnalystExists {
					//Get analyst from cache as exists
					analystIsInCache, strOwnerName, _ := recordInCache(strOwnerID, "Analyst")
					if analystIsInCache && strOwnerName != "" {
						coreFields[strAttribute] = strOwnerID
						coreFields["h_ownername"] = strOwnerName
					}
				}
			}

			if strOwnerName == "" && mapGenericConf.DefaultOwner != "" {
				strOwnerID = mapGenericConf.DefaultOwner
				boolAnalystExists := doesAnalystExist(strOwnerID, espXmlmc, buffer)
				if boolAnalystExists {
					//Get customer from user cache as exists
					analystIsInCache, strOwnerName, _ := recordInCache(strOwnerID, "Analyst")
					if analystIsInCache && strOwnerName != "" {
						coreFields[strAttribute] = strOwnerID
						coreFields["h_ownername"] = strOwnerName
					}
				}
			}
			boolAutoProcess = false
		}

		//Customer ID & Name
		if strAttribute == "h_fk_user_id" {
			strCustID := getFieldValue(strMapping, &request.CallMap)
			if strCustID != "" {

				boolCustExists := false
				if importConf.CustomerType == 0 || importConf.CustomerType == 2 {
					//Check if customer is a User
					boolCustExists = doesAnalystExist(strCustID, espXmlmc, buffer)
					if boolCustExists {
						//Get customer from user cache as exists
						customerIsInCache, strCustName, _ := recordInCache(strCustID, "Analyst")
						if customerIsInCache && strCustName != "" {
							coreFields[strAttribute] = strCustID
							coreFields["h_fk_user_name"] = strCustName
							coreFields["h_customer_type"] = "0"
						}
					}
				}
				if !boolCustExists && (importConf.CustomerType == 1 || importConf.CustomerType == 2) {
					//Check if customer is a Contact
					_ = doesContactExist(strCustID, espXmlmc, buffer)
					//Get customer from cache as exists
					customerIsInCache, strCustName, intCustID := recordInCache(strCustID, "Customer")
					if customerIsInCache && strCustName != "" {
						coreFields[strAttribute] = strconv.Itoa(intCustID)
						coreFields["h_fk_user_name"] = strCustName
						coreFields["h_customer_type"] = "1"
					}
				}
			}
			boolAutoProcess = false
		}

		//Priority ID & Name
		//-- Get Priority ID
		if strAttribute == "h_fk_priorityid" {
			strPriorityID := getFieldValue(strMapping, &request.CallMap)
			strPriorityMapped, strPriorityName := getCallPriorityID(strPriorityID, espXmlmc, buffer)
			if strPriorityMapped == "" && mapGenericConf.DefaultPriority != "" {
				strPriorityMapped = getPriorityID(mapGenericConf.DefaultPriority, espXmlmc, buffer)
				strPriorityName = mapGenericConf.DefaultPriority
			}
			coreFields[strAttribute] = strPriorityMapped
			coreFields["h_fk_priorityname"] = strPriorityName
			boolAutoProcess = false
		}

		// Category ID & Name
		if strAttribute == "h_category_id" && strMapping != "" {
			//-- Get Call Category ID
			strCategoryID, strCategoryName := getCallCategoryID(&request.CallMap, "Request", espXmlmc, buffer)
			if strCategoryID != "" && strCategoryName != "" {
				coreFields[strAttribute] = strCategoryID
				coreFields["h_category"] = strCategoryName
			}
			boolAutoProcess = false
		}

		// Closure Category ID & Name
		if strAttribute == "h_closure_category_id" && strMapping != "" {
			strClosureCategoryID, strClosureCategoryName := getCallCategoryID(&request.CallMap, "Closure", espXmlmc, buffer)
			if strClosureCategoryID != "" {
				coreFields[strAttribute] = strClosureCategoryID
				coreFields["h_closure_category"] = strClosureCategoryName
			}
			boolAutoProcess = false
		}

		// Service ID & Name, & BPM Workflow
		if strAttribute == "h_fk_serviceid" {
			//-- Get Service ID
			appServiceID := getFieldValue(strMapping, &request.CallMap)
			strServiceID := getCallServiceID(appServiceID, espXmlmc, buffer)
			useDefaultService := false
			if strServiceID == "" && mapGenericConf.DefaultService != "" {
				useDefaultService = true
				strServiceID = getServiceID(mapGenericConf.DefaultService, espXmlmc, buffer)
			}
			if strServiceID != "" {
				//-- Get record from Service Cache
				strServiceName := ""
				strCatalogName := ""
				strCatalogID := ""

				mutexServices.Lock()
				for _, service := range services {
					if strconv.Itoa(service.ServiceID) == strServiceID {
						strServiceName = service.ServiceName

						if useDefaultService && request.GenericImportConf.DefaultCatalog != 0 {
							//Default Catalog match?
							for _, catalog := range service.CatalogItems {
								if catalog.CatalogItemID == request.GenericImportConf.DefaultCatalog &&
									(catalog.Status == "publish" || importConf.AllowUnpublishedCatalogItems) &&
									catalog.RequestType == request.GenericImportConf.ServiceManagerRequestType {
									strCatalogID = strconv.Itoa(catalog.CatalogItemID)
									strCatalogName = catalog.CatalogItemName
									strServiceBPM = catalog.BPM
								}
							}
						} else {
							//Check catalog match
							if strCatalogNameMapping, ok := mapGenericConf.CoreFieldMapping["h_catalog_id"]; ok {
								appCatalogID := getFieldValue(fmt.Sprintf("%s", strCatalogNameMapping), &request.CallMap)
								if importConf.ServiceCatalogItemMapping[appCatalogID] != 0 {
									for _, catalog := range service.CatalogItems {
										if catalog.CatalogItemID == importConf.ServiceCatalogItemMapping[appCatalogID] &&
											(catalog.Status == "publish" || importConf.AllowUnpublishedCatalogItems) &&
											catalog.RequestType == request.GenericImportConf.ServiceManagerRequestType {
											strCatalogID = strconv.Itoa(catalog.CatalogItemID)
											strCatalogName = catalog.CatalogItemName
											strServiceBPM = catalog.BPM
										}
									}
								}
							}
						}

						if strServiceBPM == "" {
							switch request.GenericImportConf.ServiceManagerRequestType {
							case "Incident":
								strServiceBPM = service.ServiceBPMIncident
							case "Service Request":
								strServiceBPM = service.ServiceBPMService
							case "Change Request":
								strServiceBPM = service.ServiceBPMChange
							case "Problem":
								strServiceBPM = service.ServiceBPMProblem
							case "Known Error":
								strServiceBPM = service.ServiceBPMKnownError
							case "Release":
								strServiceBPM = service.ServiceBPMRelease
							}
						}
					}
				}
				mutexServices.Unlock()

				if strServiceName != "" {
					buffer.WriteString(loggerGen(1, "Using Service ID "+strServiceID+" ["+strServiceName+"]"))
					coreFields[strAttribute] = strServiceID
					coreFields["h_fk_servicename"] = strServiceName
				} else {
					buffer.WriteString(loggerGen(1, "No matching Service found."))
				}
				if strCatalogName != "" {
					buffer.WriteString(loggerGen(1, "Using Catalog ID "+strCatalogID+" ["+strCatalogName+"]"))
					coreFields["h_catalog"] = strCatalogName
					coreFields["h_catalog_id"] = strCatalogID
				} else {
					buffer.WriteString(loggerGen(1, "No matching Catalog Item found."))
				}
				if strServiceBPM != "" {
					buffer.WriteString(loggerGen(1, "Using BPM "+strServiceBPM))
				} else {
					buffer.WriteString(loggerGen(1, "No matching BPM found."))
				}
			}
			boolAutoProcess = false
		}

		// Team ID and Name
		if strAttribute == "h_fk_team_id" {
			//-- Get Team ID
			appTeamID := getFieldValue(strMapping, &request.CallMap)
			strTeamID, strTeamName := getCallTeamID(appTeamID, espXmlmc, buffer)
			if strTeamID == "" && mapGenericConf.DefaultTeam != "" {
				strTeamName = mapGenericConf.DefaultTeam
				strTeamID = getTeamID(strTeamName, espXmlmc, buffer)
			}
			if strTeamID != "" && strTeamName != "" {
				coreFields[strAttribute] = strTeamID
				coreFields["h_fk_team_name"] = strTeamName
			}
			boolAutoProcess = false
		}

		// Site ID and Name
		if strAttribute == "h_site_id" {
			//-- Get site ID
			siteID, siteName := getSiteID(&request.CallMap, espXmlmc, buffer)
			if siteID != "" && siteName != "" {
				coreFields[strAttribute] = siteID
				coreFields["h_site"] = siteName
			}
			boolAutoProcess = false
		}

		// Resolved Date/Time
		if strAttribute == "h_dateresolved" && strMapping != "" && (strStatus == "status.resolved" || strStatus == "status.closed") {
			strResolvedDate := getFieldValue(strMapping, &request.CallMap)
			if strResolvedDate != "" {
				coreFields[strAttribute] = parseDateTime(strResolvedDate, strAttribute, buffer)
			}
		}

		// Closed Date/Time
		if strAttribute == "h_dateclosed" && strMapping != "" {
			strClosedDate = getFieldValue(strMapping, &request.CallMap)
			if strClosedDate != "" && strStatus != "status.onHold" {
				coreFields[strAttribute] = parseDateTime(strClosedDate, strAttribute, buffer)
			}
			if strClosedDate != "" && strStatus == "status.onHold" {
				strClosedDate = parseDateTime(strClosedDate, strAttribute, buffer)
			}
		}

		// Request Status
		if strAttribute == "h_status" {
			if strStatus == "status.onHold" {
				strStatus = "status.open"
				boolOnHoldRequest = true
			}
			coreFields[strAttribute] = strStatus
			boolAutoProcess = false
		}

		// Log Date/Time - setup ready to be processed after call logged
		if strAttribute == "h_datelogged" && strMapping != "" {
			strLoggedDate = parseDateTime(getFieldValue(strMapping, &request.CallMap), strAttribute, buffer)

			if strLoggedDate != "" {
				boolUpdateLogDate = true
			}
		}

		//Everything Else
		if boolAutoProcess &&
			strAttribute != "h_status" &&
			strAttribute != "h_requesttype" &&
			strAttribute != "h_request_prefix" &&
			strAttribute != "h_category" &&
			strAttribute != "h_closure_category" &&
			strAttribute != "h_fk_servicename" &&
			strAttribute != "h_fk_serviceid" &&
			strAttribute != "h_catalog_id" &&
			strAttribute != "h_catalog" &&
			strAttribute != "h_fk_team_name" &&
			strAttribute != "h_site" &&
			strAttribute != "h_fk_priorityname" &&
			strAttribute != "h_ownername" &&
			strAttribute != "h_fk_user_id" &&
			strAttribute != "h_fk_user_name" &&
			strAttribute != "h_datelogged" &&
			strAttribute != "h_dateresolved" &&
			strAttribute != "h_dateclosed" &&
			strAttribute != "h_customer_type" {

			if strMapping != "" && getFieldValue(strMapping, &request.CallMap) != "" {
				coreFields[strAttribute] = getFieldValue(strMapping, &request.CallMap)
			}
		}

	}

	espXmlmc.SetParam("application", appServiceManager)
	espXmlmc.SetParam("entity", "Requests")
	espXmlmc.SetParam("returnModifiedData", "true")
	espXmlmc.OpenElement("primaryEntityData")
	espXmlmc.OpenElement("record")

	for k, v := range coreFields {
		espXmlmc.SetParam(k, v)
	}

	//Add request class & prefix
	espXmlmc.SetParam("h_requesttype", request.GenericImportConf.ServiceManagerRequestType)
	espXmlmc.SetParam("h_request_prefix", reqPrefix)
	espXmlmc.CloseElement("record")
	espXmlmc.CloseElement("primaryEntityData")

	//Class Specific Data Insert
	espXmlmc.OpenElement("relatedEntityData")
	espXmlmc.SetParam("relationshipName", "Call Type")
	espXmlmc.SetParam("entityAction", "insert")
	espXmlmc.OpenElement("record")
	strAttribute = ""
	strMapping = ""
	//Loop through AdditionalFieldMapping fields from config, add to XMLMC Params if not empty
	for k, v := range mapGenericConf.AdditionalFieldMapping {
		strAttribute = fmt.Sprintf("%v", k)
		strMapping = fmt.Sprintf("%v", v)
		if strMapping != "" && getFieldValue(strMapping, &request.CallMap) != "" {
			espXmlmc.SetParam(strAttribute, getFieldValue(strMapping, &request.CallMap))
		}
	}

	espXmlmc.CloseElement("record")
	espXmlmc.CloseElement("relatedEntityData")

	//Extended Data Insert
	espXmlmc.OpenElement("relatedEntityData")
	espXmlmc.SetParam("relationshipName", "Extended Information")
	espXmlmc.SetParam("entityAction", "insert")
	espXmlmc.OpenElement("record")
	espXmlmc.SetParam("h_request_type", request.GenericImportConf.ServiceManagerRequestType)
	strAttribute = ""
	strMapping = ""
	//Loop through AdditionalFieldMapping fields from config, add to XMLMC Params if not empty
	for k, v := range mapGenericConf.AdditionalFieldMapping {
		strAttribute = fmt.Sprintf("%v", k)
		strSubString := "h_custom_"
		if strings.Contains(strAttribute, strSubString) {
			strAttribute = convExtendedColName(strAttribute)
			strMapping = fmt.Sprintf("%s", v)
			if strMapping != "" && getFieldValue(strMapping, &request.CallMap) != "" {
				espXmlmc.SetParam(strAttribute, getFieldValue(strMapping, &request.CallMap))
			}
		}
	}

	espXmlmc.CloseElement("record")
	espXmlmc.CloseElement("relatedEntityData")

	//-- Check for Dry Run
	if !configDryRun {
		XMLRequest := espXmlmc.GetParam()
		XMLCreate, xmlmcErr := espXmlmc.Invoke("data", "entityAddRecord")
		if xmlmcErr != nil {

			mutexCounters.Lock()
			counters.createdSkipped++
			mutexCounters.Unlock()
			buffer.WriteString(loggerGen(4, "API Call Failed: New Request : "+xmlmcErr.Error()))
			buffer.WriteString(loggerGen(1, "[XML] "+XMLRequest))
			return oldReference, newReference, oldGUID
		}
		var xmlRespon xmlmcRequestResponseStruct

		err := xml.Unmarshal([]byte(XMLCreate), &xmlRespon)
		if err != nil {
			mutexCounters.Lock()
			counters.createdSkipped++
			mutexCounters.Unlock()
			buffer.WriteString(loggerGen(4, "Response Unmarshal failed: New Request : "+fmt.Sprintf("%v", err)))
			buffer.WriteString(loggerGen(1, "[XML] "+XMLRequest))
			return oldReference, newReference, oldGUID
		}
		if xmlRespon.MethodResult != "ok" {
			mutexCounters.Lock()
			counters.createdSkipped++
			mutexCounters.Unlock()
			buffer.WriteString(loggerGen(4, "MethodResult not OK: New Request : "+xmlRespon.State.ErrorRet))
			buffer.WriteString(loggerGen(1, "[XML] "+XMLRequest))
			return oldReference, newReference, oldGUID
		}

		newReference = xmlRespon.RequestID

		mutexCounters.Lock()
		counters.created++
		mutexCounters.Unlock()

		buffer.WriteString(loggerGen(1, "New Service Manager Request: "+newReference))

		createActivityStream(newReference, espXmlmc, buffer)

		//Now update Logdate
		if boolUpdateLogDate {
			updateLogDate(newReference, strLoggedDate, espXmlmc, buffer)
		}

		//Now do BPM Processing
		if strStatus != "status.resolved" &&
			strStatus != "status.closed" &&
			strStatus != "status.cancelled" {
			if newReference != "" && strServiceBPM != "" {
				mutexCounters.Lock()
				counters.bpmAvailable++
				mutexCounters.Unlock()
				arrSpawnBPMs = append(arrSpawnBPMs, spawnBPMStruct{RequestID: newReference, BPMID: strServiceBPM})
			}
		}

		// Now handle calls in an On Hold status
		if boolOnHoldRequest {
			holdRequest(newReference, strClosedDate, espXmlmc, buffer)
		}

		if request.GenericImportConf.ParentRequestRefColumn != "" {
			parentRef := getFieldValue("["+request.GenericImportConf.ParentRequestRefColumn+"]", &request.CallMap)
			if parentRef != "" {
				buffer.WriteString(loggerGen(1, "Parent Request Reference: ["+parentRef+"]"))
				linkRequests(parentRef, newReference, espXmlmc, buffer)
			} else {
				buffer.WriteString(loggerGen(5, "Could not retrieve a Parent Reference value from column ["+request.GenericImportConf.ParentRequestRefColumn+"]"))
			}
		}
	} else {
		//-- DEBUG XML TO LOG FILE
		var XMLSTRING = espXmlmc.GetParam()
		buffer.WriteString(loggerGen(1, "Request Log XML "+XMLSTRING))
		mutexCounters.Lock()
		counters.createdSkipped++
		mutexCounters.Unlock()
		espXmlmc.ClearParam()
	}
	return oldReference, newReference, oldGUID
}

func linkRequests(parentRef, newRef string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) {
	postVisibility := "trustedGuest"
	if importConf.LinkedRequestPostVilibility == "team" {
		postVisibility = "colleague"
	}
	espXmlmc.SetParam("entityId", newRef)
	espXmlmc.SetParam("entityName", "Requests")
	espXmlmc.SetParam("linkedEntityId", parentRef)
	espXmlmc.SetParam("linkedEntityName", "Requests")
	espXmlmc.SetParam("updateTimeline", "true")
	espXmlmc.SetParam("visibility", postVisibility)
	XMLHold := espXmlmc.GetParam()
	XMLBPM, xmlmcErr := espXmlmc.Invoke("apps/com.hornbill.servicemanager/RelationshipEntities", "add")
	if xmlmcErr != nil {
		buffer.WriteString(loggerGen(4, "XMLMC error: Unable to link ["+newRef+"] to ["+parentRef+"] : "+fmt.Sprintf("%v", xmlmcErr)))
		buffer.WriteString(loggerGen(1, XMLHold))
		return
	}
	var xmlRespon xmlmcResponse

	errLogDate := xml.Unmarshal([]byte(XMLBPM), &xmlRespon)
	if errLogDate != nil {
		buffer.WriteString(loggerGen(4, "Unmarshal error: Unable to link ["+newRef+"] to ["+parentRef+"] : "+fmt.Sprintf("%v", errLogDate)))
		buffer.WriteString(loggerGen(1, XMLHold))
		return
	}
	if xmlRespon.MethodResult != "ok" {
		buffer.WriteString(loggerGen(4, "MethodResult not OK: Unable to link ["+newRef+"] to ["+parentRef+"] : "+xmlRespon.State.ErrorRet))
		buffer.WriteString(loggerGen(1, XMLHold))
		return
	}
	buffer.WriteString(loggerGen(1, "Requests Linked Successfully: ["+newRef+"] to ["+parentRef+"] "))
}

func holdRequest(newCallRef, holdDate string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) {
	espXmlmc.SetParam("requestId", newCallRef)
	espXmlmc.SetParam("onHoldUntil", holdDate)
	espXmlmc.SetParam("strReason", "Request imported in an On Hold status. See Historical Request Updates for further information.")
	XMLHold := espXmlmc.GetParam()
	XMLBPM, xmlmcErr := espXmlmc.Invoke("apps/"+appServiceManager+"/Requests", "holdRequest")
	if xmlmcErr != nil {
		buffer.WriteString(loggerGen(4, "XMLMC error: Unable to place request on hold ["+newCallRef+"] : "+fmt.Sprintf("%v", xmlmcErr)))
		buffer.WriteString(loggerGen(1, XMLHold))
		return
	}
	var xmlRespon xmlmcResponse

	errLogDate := xml.Unmarshal([]byte(XMLBPM), &xmlRespon)
	if errLogDate != nil {
		buffer.WriteString(loggerGen(4, "Unmarshal error: Unable to place request on hold ["+newCallRef+"] : "+fmt.Sprintf("%v", errLogDate)))
		buffer.WriteString(loggerGen(1, XMLHold))
		return
	}
	if xmlRespon.MethodResult != "ok" {
		buffer.WriteString(loggerGen(4, "MethodResult not OK: Unable to place request on hold ["+newCallRef+"] : "+xmlRespon.State.ErrorRet))
		buffer.WriteString(loggerGen(1, XMLHold))
		return
	}
	buffer.WriteString(loggerGen(1, "Request On-Hold Success"))
}

func spawnBPM(jobs chan spawnBPMStruct, wg *sync.WaitGroup, espXmlmc *apiLib.XmlmcInstStruct) {
	defer wg.Done()
	for requestRecord := range jobs {
		var buffer bytes.Buffer
		buffer.WriteString(loggerGen(3, "   "))
		buffer.WriteString(loggerGen(1, "Spawning BPM ["+requestRecord.BPMID+"] for ["+requestRecord.RequestID+"]"))
		espXmlmc.SetParam("application", appServiceManager)
		espXmlmc.SetParam("name", requestRecord.BPMID)
		espXmlmc.SetParam("reference", "urn:sys:entity:"+appServiceManager+":Requests:"+requestRecord.RequestID)
		espXmlmc.OpenElement("inputParam")
		espXmlmc.SetParam("name", "requestId")
		espXmlmc.SetParam("value", requestRecord.RequestID)
		espXmlmc.CloseElement("inputParam")
		XMLBPM, xmlmcErr := espXmlmc.Invoke("bpm", "processSpawn2")
		if xmlmcErr != nil {
			buffer.WriteString(loggerGen(4, "API Call Failed: Spawn BPM: "+fmt.Sprintf("%v", xmlmcErr)))
			bufferMutex.Lock()
			loggerWriteBuffer(buffer.String())
			bufferMutex.Unlock()
			buffer.Reset()
			continue
		}
		var xmlRespon xmlmcBPMSpawnedStruct

		errBPM := xml.Unmarshal([]byte(XMLBPM), &xmlRespon)
		if errBPM != nil {
			buffer.WriteString(loggerGen(4, "Response Unmarshal Failed: Spawn BPM: "+fmt.Sprintf("%v", errBPM)))
			bufferMutex.Lock()
			loggerWriteBuffer(buffer.String())
			bufferMutex.Unlock()
			buffer.Reset()
			continue
		}
		if xmlRespon.MethodResult != "ok" {
			buffer.WriteString(loggerGen(4, "MethodResult not OK: Spawn BPM: "+xmlRespon.State.ErrorRet))
			bufferMutex.Lock()
			loggerWriteBuffer(buffer.String())
			bufferMutex.Unlock()
			buffer.Reset()
			continue
		}
		mutexCounters.Lock()
		counters.bpmSpawned++
		mutexCounters.Unlock()
		buffer.WriteString(loggerGen(1, "BPM Spawned: "+xmlRespon.Identifier))
		updateRequestBpm(requestRecord.RequestID, xmlRespon.Identifier, espXmlmc, &buffer)

		bufferMutex.Lock()
		loggerWriteBuffer(buffer.String())
		bufferMutex.Unlock()
		buffer.Reset()
	}
}

func updateRequestBpm(newCallRef, bpmID string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) {
	espXmlmc.SetParam("application", appServiceManager)
	espXmlmc.SetParam("table", "h_itsm_requests")
	espXmlmc.OpenElement("recordData")
	espXmlmc.SetParam("keyValue", newCallRef)
	espXmlmc.OpenElement("record")
	espXmlmc.SetParam("h_bpm_id", bpmID)
	espXmlmc.CloseElement("record")
	espXmlmc.CloseElement("recordData")

	XMLBPMUpdate, xmlmcErr := espXmlmc.Invoke("data", "updateRecord")
	if xmlmcErr != nil {
		buffer.WriteString(loggerGen(4, "API Call Failed: Associate BPM to Request: "+fmt.Sprintf("%v", xmlmcErr)))
		return
	}
	var xmlRespon xmlmcResponse
	errBPMSpawn := xml.Unmarshal([]byte(XMLBPMUpdate), &xmlRespon)
	if errBPMSpawn != nil {
		buffer.WriteString(loggerGen(4, "Response Unmarshal Failed: Associate BPM to Request: "+fmt.Sprintf("%v", errBPMSpawn)))
		return
	}
	if xmlRespon.MethodResult != "ok" {
		buffer.WriteString(loggerGen(4, "MethodResult not OK: Associate BPM to Request: "+xmlRespon.State.ErrorRet))
		return
	}

	buffer.WriteString(loggerGen(1, "BPM ["+bpmID+"] Associated to Request ["+newCallRef+"]"))
	mutexCounters.Lock()
	counters.bpmRequest++
	mutexCounters.Unlock()
}

func updateLogDate(newCallRef, logDate string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) {
	espXmlmc.SetParam("application", appServiceManager)
	espXmlmc.SetParam("entity", "Requests")
	espXmlmc.OpenElement("primaryEntityData")
	espXmlmc.OpenElement("record")
	espXmlmc.SetParam("h_pk_reference", newCallRef)
	espXmlmc.SetParam("h_datelogged", logDate)
	espXmlmc.CloseElement("record")
	espXmlmc.CloseElement("primaryEntityData")
	XMLLogDate, xmlmcErr := espXmlmc.Invoke("data", "entityUpdateRecord")
	if xmlmcErr != nil {
		buffer.WriteString(loggerGen(4, "API Call Failed: Update Log Date ["+newCallRef+"] : "+fmt.Sprintf("%v", xmlmcErr)))
		return
	}
	var xmlRespon xmlmcResponse
	errLogDate := xml.Unmarshal([]byte(XMLLogDate), &xmlRespon)
	if errLogDate != nil {
		buffer.WriteString(loggerGen(4, "Unmarshall Response Failed: Update Log Date ["+newCallRef+"] : "+fmt.Sprintf("%v", errLogDate)))
		return
	}
	if xmlRespon.MethodResult != "ok" {
		buffer.WriteString(loggerGen(4, "MethodResult Not OK: Update Log Date ["+newCallRef+"] : "+xmlRespon.State.ErrorRet))
		return
	}
	buffer.WriteString(loggerGen(1, "Request Log Date Update Successful"))
}

func createActivityStream(newCallRef string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) {
	//Now update the request to create the activity stream
	espXmlmc.SetParam("socialObjectRef", "urn:sys:entity:"+appServiceManager+":Requests:"+newCallRef)
	espXmlmc.SetParam("content", "Request Imported")
	espXmlmc.SetParam("visibility", "public")
	espXmlmc.SetParam("type", "Logged")
	fixed, err := espXmlmc.Invoke("activity", "postMessage")
	if err != nil {
		buffer.WriteString(loggerGen(5, "API Call Failed: Activity Stream Creation ["+newCallRef+"] : "+fmt.Sprintf("%v", err)))
		return
	}
	var xmlRespon xmlmcResponse
	err = xml.Unmarshal([]byte(fixed), &xmlRespon)
	if err != nil {
		buffer.WriteString(loggerGen(5, "Unmarshall Response Failed: Activity Stream Creation ["+newCallRef+"] : "+fmt.Sprintf("%v", err)))
		return
	}
	if xmlRespon.MethodResult != "ok" {
		buffer.WriteString(loggerGen(5, "MethodResult Not OK: Activity Stream Creation ["+newCallRef+"] : "+xmlRespon.State.ErrorRet))
		return
	}

	buffer.WriteString(loggerGen(1, "Activity Stream Creation Successful"))
}
