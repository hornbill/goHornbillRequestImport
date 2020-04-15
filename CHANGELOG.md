# CHANGELOG

## 1.7.2 (April 15th, 2020)

### Change

- Updated code to support Core application and platform changes

## 1.7.1

### Defect Fix

- Fixed error when no historic update query is provided

## 1.7.0

### Feature

- Added the ability to link imported Requests with a per-row Parent Request

## 1.6.0

### Features

- On tool completion, outputs a total of how many request records were found
- Better error handling and logging when checking records to be imported

## 1.5.0

### Feature

- Added support to assign Cataog Items in a non-published state to imported requests

### Defect Fixes

- Fixed issue with int32 datatypes not correctly being extracted from data source record columns
- Fixed issue with spawning BPMs against imported requests

## 1.4.0

### Features

- When creating historic update records, if the user is not able to be resolved on the Hornbill instance then the literal values provided will be used instead (in both the h_updateby and h_updatebyname properties)
- When creating historic update records, the literal value of group will be written to h_updatebygroup, if this field is mapped. Removed the default group value populating this column, as this was not logical

## 1.3.1 (May 28th 2019)

### Defect Fix

- Fixed duplicate parsing of h_dateclosed field mapping

## 1.3.0 (March 21st 2019)

### Features

- Added support to import Releases
- Added MySQL 8.x support
- Map Catalog Items for Services, and to use the Catalog Item BPM workflow should one exist
- Can now set default Catalog Item if default Service is applied
- Can now set default owner alongside default team
- Added version flag for cross-compiling script support

## 1.2.0 (January 4th 2019)

### Features

- Added catch for duplicate request records to help with DISTINCT limitations in the Microsoft Excel ODBC driver
- Improved datetime parsing, including new config parameter to allow the specification of the expected datetime string layout returned by the query

## 1.1.0 (December 12th 2018)

### Feature

- Added support to search both Contacts and Users for request customer, rather than just one or the other

## 1.0.0 (November 21st 2018)

### Initial Release
