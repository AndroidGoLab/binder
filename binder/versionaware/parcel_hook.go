package versionaware

import "github.com/AndroidGoLab/binder/parcel"

func init() {
	parcel.DetectAPILevel = DetectAPILevel
}
