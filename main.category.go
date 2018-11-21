package main

import (
	"bytes"
	"encoding/xml"
	"fmt"

	"github.com/hornbill/goApiLib"
)

//getCallCategoryID takes the Call Record and returns a correct Category ID if one exists on the Instance
func getCallCategoryID(callMap *map[string]interface{}, categoryGroup string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) (string, string) {
	categoryID := ""
	categoryString := ""
	categoryNameMapping := ""
	categoryCode := ""
	if categoryGroup == "Request" {
		categoryNameMapping = fmt.Sprintf("%v", mapGenericConf.CoreFieldMapping["h_category_id"])
		categoryCode = getFieldValue(categoryNameMapping, callMap)
		if importConf.CategoryMapping[categoryCode] != nil {
			//Get Category Code from JSON mapping
			categoryCode = fmt.Sprintf("%s", importConf.CategoryMapping[categoryCode])
		}
	} else {
		categoryNameMapping = fmt.Sprintf("%v", mapGenericConf.CoreFieldMapping["h_closure_category_id"])
		categoryCode = getFieldValue(categoryNameMapping, callMap)
		if importConf.ResolutionCategoryMapping[categoryCode] != nil {
			//Get Category Code from JSON mapping
			categoryCode = fmt.Sprintf("%s", importConf.ResolutionCategoryMapping[categoryCode])
		}
	}
	if categoryCode != "" {
		categoryID, categoryString = getCategoryID(categoryCode, categoryGroup, espXmlmc, buffer)
	}
	return categoryID, categoryString
}

//getCategoryID takes a Category Code string and returns a correct Category ID if one exists in the cache or on the Instance
func getCategoryID(categoryCode, categoryGroup string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) (string, string) {

	categoryID := ""
	categoryString := ""
	if categoryCode != "" {
		categoryIsInCache, CategoryIDCache, CategoryNameCache := categoryInCache(categoryCode, categoryGroup+"Category")
		//-- Check if we have cached the Category already
		if categoryIsInCache {
			categoryID = CategoryIDCache
			categoryString = CategoryNameCache
		} else {
			categoryIsOnInstance, CategoryIDInstance, CategoryStringInstance := searchCategory(categoryCode, categoryGroup, espXmlmc, buffer)
			//-- If Returned set output
			if categoryIsOnInstance {
				categoryID = CategoryIDInstance
				categoryString = CategoryStringInstance
			}
		}
	}
	return categoryID, categoryString
}

// seachCategory -- Function to check if passed-through support category name is on the instance
func searchCategory(categoryCode, categoryGroup string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) (bool, string, string) {
	boolReturn := false
	idReturn := ""
	strReturn := ""
	//-- ESP Query for category
	espXmlmc.SetParam("codeGroup", categoryGroup)
	espXmlmc.SetParam("code", categoryCode)
	XMLCategorySearch, xmlmcErr := espXmlmc.Invoke("data", "profileCodeLookup")
	if xmlmcErr != nil {
		buffer.WriteString(loggerGen(4, "API Call Failed: Category Search: ["+categoryGroup+"] ["+categoryCode+"]: "+fmt.Sprintf("%v", xmlmcErr)))
		return boolReturn, idReturn, strReturn
	}
	var xmlRespon xmlmcCategoryListResponse

	err := xml.Unmarshal([]byte(XMLCategorySearch), &xmlRespon)
	if err != nil {
		buffer.WriteString(loggerGen(4, "Response Unmarshal Failed: Category Search: ["+categoryGroup+"] ["+categoryCode+"]: "+fmt.Sprintf("%v", err)))
	} else {
		if xmlRespon.MethodResult != "ok" {
			buffer.WriteString(loggerGen(4, "MethodResult Not OK: Category Search: ["+categoryGroup+"] ["+categoryCode+"]: "+xmlRespon.State.ErrorRet))
		} else {
			//-- Check Response
			if xmlRespon.CategoryName != "" {
				strReturn = xmlRespon.CategoryName
				idReturn = xmlRespon.CategoryID
				boolReturn = true
				//-- Add Category to Cache
				var newCategoryForCache categoryListStruct
				newCategoryForCache.CategoryID = idReturn
				newCategoryForCache.CategoryCode = categoryCode
				newCategoryForCache.CategoryName = strReturn
				categoryNamedMap := []categoryListStruct{newCategoryForCache}
				switch categoryGroup {
				case "Request":
					mutexCategories.Lock()
					categories = append(categories, categoryNamedMap...)
					mutexCategories.Unlock()
					buffer.WriteString(loggerGen(1, "Request Category Cached ["+idReturn+"]: "+strReturn))
				case "Closure":
					mutexCloseCategories.Lock()
					closeCategories = append(closeCategories, categoryNamedMap...)
					mutexCloseCategories.Unlock()
					buffer.WriteString(loggerGen(1, "Closure Category Cached ["+idReturn+"]: "+strReturn))
				}
			} else {
				buffer.WriteString(loggerGen(5, "Methodcall result OK for "+categoryGroup+" Category ["+categoryCode+"] but category name blank: ["+xmlRespon.CategoryID+"] ["+xmlRespon.CategoryName+"]"))
			}
		}
	}
	return boolReturn, idReturn, strReturn
}
