//go:build !prebuilt_tables

package versionaware

// DefaultAPILevel is the API level that the compiled proxy code was
// generated against. Without prebuilt_tables, transaction codes are
// resolved dynamically from device framework JARs.
var DefaultAPILevel int

// Tables is empty without prebuilt_tables; all resolution happens
// via lazy JAR extraction.
var Tables MultiVersionTable

// Revisions is empty without prebuilt_tables.
var Revisions = APIRevisions{}
