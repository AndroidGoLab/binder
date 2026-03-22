package camera

import (
	"context"
	"fmt"

	fwkDevice "github.com/AndroidGoLab/binder/android/frameworks/cameraservice/device"
	"github.com/AndroidGoLab/binder/binder"
	"github.com/AndroidGoLab/binder/parcel"
)

// CreateStreamWithSurface creates a camera stream with the given IGBP
// stub as the surface. Uses raw parcel I/O because the generated proxy
// does not handle the native Surface/IGBP binder embedding.
func CreateStreamWithSurface(
	ctx context.Context,
	deviceUser fwkDevice.ICameraDeviceUser,
	transport binder.Transport,
	igbpStub *binder.StubBinder,
	width int32,
	height int32,
) (int32, error) {
	data := parcel.New()
	data.WriteInterfaceToken(fwkDevice.DescriptorICameraDeviceUser)

	data.WriteInt32(1) // non-null OutputConfiguration

	headerPos := parcel.WriteParcelableHeader(data)

	data.WriteInt32(0)  // windowHandles: empty array
	data.WriteInt32(0)  // rotation: R0
	data.WriteInt32(-1) // windowGroupId: NONE
	data.WriteString16("")
	data.WriteInt32(width)
	data.WriteInt32(height)
	data.WriteBool(false) // isDeferred

	// surfaces: array of 1.
	data.WriteInt32(1)
	data.WriteInt32(1) // non-null

	// view::Surface::writeToParcel
	data.WriteString16("GoCamera")
	data.WriteInt32(0)           // isSingleBuffered
	data.WriteUint32(0x62717565) // USE_BUFFER_QUEUE
	binder.WriteBinderToParcel(ctx, data, igbpStub, transport)
	data.WriteNullStrongBinder() // surfaceControlHandle: null

	parcel.WriteParcelableFooter(data, headerPos)

	reply, err := deviceUser.AsBinder().Transact(
		ctx,
		fwkDevice.TransactionICameraDeviceUserCreateStream,
		0,
		data,
	)
	if err != nil {
		return 0, fmt.Errorf("transaction: %w", err)
	}
	defer reply.Recycle()

	if err = binder.ReadStatus(reply); err != nil {
		return 0, fmt.Errorf("status: %w", err)
	}

	streamId, err := reply.ReadInt32()
	if err != nil {
		return 0, fmt.Errorf("readStreamId: %w", err)
	}
	return streamId, nil
}

// CreateDefaultRequest calls CreateDefaultRequest using raw parcel I/O.
func CreateDefaultRequest(
	ctx context.Context,
	deviceUser fwkDevice.ICameraDeviceUser,
	templateId fwkDevice.TemplateId,
) ([]byte, error) {
	data := parcel.New()
	data.WriteInterfaceToken(fwkDevice.DescriptorICameraDeviceUser)
	data.WriteInt32(int32(templateId))

	reply, err := deviceUser.AsBinder().Transact(
		ctx,
		fwkDevice.TransactionICameraDeviceUserCreateDefaultRequest,
		0,
		data,
	)
	if err != nil {
		return nil, fmt.Errorf("transaction: %w", err)
	}
	defer reply.Recycle()

	if err = binder.ReadStatus(reply); err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}

	nullInd, err := reply.ReadInt32()
	if err != nil {
		return nil, fmt.Errorf("null indicator: %w", err)
	}
	if nullInd == 0 {
		return nil, fmt.Errorf("null metadata")
	}

	endPos, err := parcel.ReadParcelableHeader(reply)
	if err != nil {
		return nil, fmt.Errorf("parcelable header: %w", err)
	}

	metadataBytes, err := reply.ReadByteArray()
	if err != nil {
		return nil, fmt.Errorf("ReadByteArray: %w", err)
	}

	parcel.SkipToParcelableEnd(reply, endPos)
	return metadataBytes, nil
}

// SubmitRequest sends a SubmitRequestList using raw parcel I/O.
func SubmitRequest(
	ctx context.Context,
	deviceUser fwkDevice.ICameraDeviceUser,
	captureReq fwkDevice.CaptureRequest,
	isRepeating bool,
) (fwkDevice.SubmitInfo, error) {
	var result fwkDevice.SubmitInfo

	data := parcel.New()
	data.WriteInterfaceToken(fwkDevice.DescriptorICameraDeviceUser)
	data.WriteInt32(1) // requestList: array of 1
	data.WriteInt32(1) // non-null indicator
	if err := captureReq.MarshalParcel(data); err != nil {
		return result, fmt.Errorf("marshal: %w", err)
	}
	data.WriteBool(isRepeating)

	reply, err := deviceUser.AsBinder().Transact(
		ctx,
		fwkDevice.TransactionICameraDeviceUserSubmitRequestList,
		0,
		data,
	)
	if err != nil {
		return result, fmt.Errorf("transaction: %w", err)
	}
	defer reply.Recycle()

	if err = binder.ReadStatus(reply); err != nil {
		return result, fmt.Errorf("status: %w", err)
	}

	nullInd, err := reply.ReadInt32()
	if err != nil {
		return result, fmt.Errorf("null indicator: %w", err)
	}
	if nullInd != 0 {
		if err = result.UnmarshalParcel(reply); err != nil {
			return result, fmt.Errorf("unmarshal SubmitInfo: %w", err)
		}
	}

	return result, nil
}
