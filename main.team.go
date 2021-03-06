package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	apiLib "github.com/hornbill/goApiLib"
)

//getCallTeamID takes the Call Record and returns a correct Team ID if one exists on the Instance
func getCallTeamID(swTeamID string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) (string, string) {
	teamID := ""
	teamName := ""
	if importConf.TeamMapping[swTeamID] != nil {
		teamName = fmt.Sprintf("%s", importConf.TeamMapping[swTeamID])
		if teamName != "" {
			teamID = getTeamID(teamName, espXmlmc, buffer)
		}
	}
	return teamID, teamName
}

//getTeamID takes a Team Name string and returns a correct Team ID if one exists in the cache or on the Instance
func getTeamID(teamName string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) string {
	teamID := ""
	if teamName != "" {
		teamIsInCache, TeamIDCache, _ := recordInCache(teamName, "Team")
		//-- Check if we have cached the Team already
		if teamIsInCache {
			teamID = TeamIDCache
		} else {
			teamIsOnInstance, TeamIDInstance := searchTeam(teamName, espXmlmc, buffer)
			//-- If Returned set output
			if teamIsOnInstance {
				teamID = TeamIDInstance
			}
		}
	}
	return teamID
}

// searchTeam -- Function to check if passed-through support team name is on the instance
func searchTeam(teamName string, espXmlmc *apiLib.XmlmcInstStruct, buffer *bytes.Buffer) (bool, string) {
	boolReturn := false
	strReturn := ""
	//-- ESP Query for team
	espXmlmc.SetParam("application", "com.hornbill.servicemanager")
	espXmlmc.SetParam("entity", "Team")
	espXmlmc.SetParam("matchScope", "all")
	espXmlmc.OpenElement("searchFilter")
	espXmlmc.SetParam("column", "h_name")
	espXmlmc.SetParam("value", teamName)
	espXmlmc.SetParam("matchType", "exact")
	espXmlmc.CloseElement("searchFilter")
	espXmlmc.OpenElement("searchFilter")
	espXmlmc.SetParam("column", "h_type")
	espXmlmc.SetParam("value", "1")
	espXmlmc.SetParam("matchType", "exact")
	espXmlmc.CloseElement("searchFilter")
	espXmlmc.SetParam("maxResults", "1")

	XMLTeamSearch, xmlmcErr := espXmlmc.Invoke("data", "entityBrowseRecords2")
	if xmlmcErr != nil {
		buffer.WriteString(loggerGen(4, "API Call Failed: Search Team ["+teamName+"]: "+fmt.Sprintf("%v", xmlmcErr)))
		return boolReturn, strReturn
	}
	var xmlRespon xmlmcTeamListResponse

	err := xml.Unmarshal([]byte(XMLTeamSearch), &xmlRespon)
	if err != nil {
		buffer.WriteString(loggerGen(4, "Response Unmarshal Failed: Search Team ["+teamName+"]: "+fmt.Sprintf("%v", err)))
		return boolReturn, strReturn
	}
	if xmlRespon.MethodResult != "ok" {
		buffer.WriteString(loggerGen(5, "MethodResult Not OK: Search Team ["+teamName+"]: "+xmlRespon.State.ErrorRet))
		return boolReturn, strReturn
	}
	//-- Check Response
	if xmlRespon.TeamName != "" {
		if strings.EqualFold(xmlRespon.TeamName, teamName) {
			strReturn = xmlRespon.TeamID
			boolReturn = true
			//-- Add Team to Cache
			var newTeamForCache teamListStruct
			newTeamForCache.TeamID = strReturn
			newTeamForCache.TeamName = teamName
			teamNamedMap := []teamListStruct{newTeamForCache}
			mutexTeams.Lock()
			teams = append(teams, teamNamedMap...)
			mutexTeams.Unlock()
			buffer.WriteString(loggerGen(1, "Team Cached ["+teamName+"] ["+strReturn+"]"))
		}
	}

	return boolReturn, strReturn
}
