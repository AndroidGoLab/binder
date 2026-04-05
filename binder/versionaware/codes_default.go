package versionaware

// DefaultAPILevel is the API level that the compiled proxy code was
// generated against. Transaction codes are resolved dynamically from
// device framework JARs.
var DefaultAPILevel int

// Tables holds compiled version tables. Empty by default; all
// resolution happens via lazy JAR extraction from the device.
var Tables MultiVersionTable

// Revisions maps API level -> list of version IDs (latest first).
var Revisions = APIRevisions{}
