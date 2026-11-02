package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/AndroidGoLab/binder/android/gui"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/binder/versionaware"
	"github.com/AndroidGoLab/binder/kernelbinder"
	"github.com/AndroidGoLab/binder/servicemanager"
)

// chooseDisplay maps zero to the primary display and otherwise requires an ID.
//
// Example: choose a physical display reported by SurfaceFlinger.
func chooseDisplay(ids []uint64, requested int) (uint64, error) {
	if len(ids) == 0 {
		return 0, errors.New("no physical displays")
	}
	if requested == 0 {
		return ids[0], nil
	}
	for _, id := range ids {
		if id == uint64(requested) {
			return id, nil
		}
	}
	return 0, fmt.Errorf("display %d not found", requested)
}

// writeDisplayInfo queries the active physical display and writes JSON metadata.
//
// Example: print the primary display's dimensions and density.
func writeDisplayInfo(ctx context.Context, out io.Writer, requested int) (resultErr error) {
	driver, err := kernelbinder.Open(ctx, binder.WithMapSize(4*1024*1024))
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, driver.Close(context.Background())) }()
	tr, err := versionaware.NewTransport(ctx, driver, 0, versionaware.OptionCachePath(""))
	if err != nil {
		return err
	}
	api := tr.APILevel()
	sm := servicemanager.New(tr)
	name := "SurfaceFlinger"
	if api == 34 {
		name = "SurfaceFlingerAIDL"
	}
	remote, err := sm.GetService(ctx, servicemanager.ServiceName(name))
	if err != nil {
		return err
	}
	ids, err := gui.PhysicalDisplayIDs(ctx, remote, api)
	if err != nil {
		return err
	}
	id, err := chooseDisplay(ids, requested)
	if err != nil {
		return err
	}
	info, err := gui.GetPhysicalDisplayInfo(ctx, remote, api, id)
	if err != nil {
		return err
	}
	if info.XDPI <= 0 || info.YDPI <= 0 {
		return errors.New("display reports invalid physical DPI")
	}
	size := math.Hypot(float64(info.Width)/float64(info.XDPI), float64(info.Height)/float64(info.YDPI))
	result := struct {
		ID       int     `json:"id"`
		Width    uint32  `json:"width"`
		Height   uint32  `json:"height"`
		XDPI     float32 `json:"xdpi"`
		YDPI     float32 `json:"ydpi"`
		Size     float64 `json:"size"`
		Density  float32 `json:"density"`
		FPS      float32 `json:"fps"`
		Secure   bool    `json:"secure"`
		Rotation int     `json:"rotation"`
	}{ID: requested, Width: info.Width, Height: info.Height, XDPI: info.XDPI, YDPI: info.YDPI, Size: size, Density: info.Density, FPS: info.FPS, Secure: info.Secure, Rotation: int(info.Rotation) * 90}
	return json.NewEncoder(out).Encode(result)
}
