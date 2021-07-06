package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	apiLib "github.com/hornbill/goApiLib"
)

//getSiteID takes the Call Record and returns a correct Site ID if one exists on the Instance
func getSiteID(callMap *map[string]interface{}, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) (string, string) {
	siteID := ""
	siteNameMapping := fmt.Sprintf("%v", mapGenericConf.CoreFieldMapping["h_site_id"])
	siteName := getFieldValue(siteNameMapping, callMap)
	if siteName != "" {
		siteIsInCache, SiteIDCache, _ := recordInCache(siteName, "Site")
		//-- Check if we have cached the site already
		if siteIsInCache {
			siteID = SiteIDCache
		} else {
			siteIsOnInstance, SiteIDInstance := searchSite(siteName, espXmlmc, buffer)
			//-- If Returned set output
			if siteIsOnInstance {
				siteID = strconv.Itoa(SiteIDInstance)
			}
		}
	}
	return siteID, siteName
}

// seachSite -- Function to check if passed-through  site  name is on the instance
func searchSite(siteName string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) (bool, int) {
	boolReturn := false
	intReturn := 0
	//-- ESP Query for site
	espXmlmc.SetParam("application", "com.hornbill.core")
	espXmlmc.SetParam("entity", "Site")
	espXmlmc.SetParam("matchScope", "all")
	espXmlmc.OpenElement("searchFilter")
	espXmlmc.SetParam("h_site_name", siteName)
	espXmlmc.CloseElement("searchFilter")
	espXmlmc.SetParam("maxResults", "1")

	XMLSiteSearch, xmlmcErr := espXmlmc.Invoke("data", "entityBrowseRecords")
	if xmlmcErr != nil {
		buffer.WriteString(loggerGen(4, "API Call Failed: Search Site ["+siteName+"]: "+fmt.Sprintf("%v", xmlmcErr)))
		return boolReturn, intReturn
	}
	var xmlRespon xmlmcSiteListResponse

	err := xml.Unmarshal([]byte(XMLSiteSearch), &xmlRespon)
	if err != nil {
		buffer.WriteString(loggerGen(4, "Response Unmarshal Failed: Search Site ["+siteName+"]: "+fmt.Sprintf("%v", err)))
		return boolReturn, intReturn
	}
	if xmlRespon.MethodResult != "ok" {
		buffer.WriteString(loggerGen(5, "MethodResult Not OK: Search Service ["+siteName+"]: "+xmlRespon.State.ErrorRet))
		return boolReturn, intReturn
	}
	//-- Check Response
	if xmlRespon.SiteName != "" {
		if strings.EqualFold(xmlRespon.SiteName, siteName) {
			intReturn = xmlRespon.SiteID
			boolReturn = true
			//-- Add Site to Cache
			var newSiteForCache siteListStruct
			newSiteForCache.SiteID = intReturn
			newSiteForCache.SiteName = siteName
			siteNamedMap := []siteListStruct{newSiteForCache}
			mutexSites.Lock()
			sites = append(sites, siteNamedMap...)
			mutexSites.Unlock()
			buffer.WriteString(loggerGen(1, "Site Cached ["+siteName+"] ["+strconv.Itoa(xmlRespon.SiteID)+"]"))
		}
	}
	return boolReturn, intReturn
}
