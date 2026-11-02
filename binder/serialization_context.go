package binder

import (
	"fmt"

	"github.com/AndroidGoLab/binder/parcel"
)

// NewBinderSerializationContext binds local Binder serialization to a transport.
func NewBinderSerializationContext(transport Transport) parcel.BinderSerializationContext {
	return binderSerializationContext{transport: transport}
}

type binderSerializationContext struct {
	transport Transport
}

func (c binderSerializationContext) AcquireLocalBinder(owner parcel.RegisteredLocalBinder) (parcel.LocalBinderIdentity, parcel.BinderLease, error) {
	stub, ok := owner.(*StubBinder)
	if !ok || stub == nil {
		return parcel.LocalBinderIdentity{}, nil, fmt.Errorf("binder: unsupported local binder owner")
	}
	lease, err := stub.acquireSerializationLease(c.transport)
	if err != nil {
		return parcel.LocalBinderIdentity{}, nil, err
	}
	return parcel.LocalBinderIdentity{Pointer: stub.BinderPtr(), Cookie: stub.Cookie()}, lease, nil
}
