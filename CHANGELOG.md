# CHANGELOG

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

### Features

- Added support to search both Contacts and Users for request customer, rather than just one or the other

## 1.0.0 (November 21st 2018)

### Initial Release