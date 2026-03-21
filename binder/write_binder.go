package binder

import (
	"context"

	"github.com/xaionaro-go/binder/parcel"
)

// WriteBinderToParcel writes an IBinder to a parcel, choosing the correct
// wire format based on whether the binder is a local stub or a remote proxy.
//
// For remote proxies (ProxyBinder), it writes BINDER_TYPE_HANDLE with the handle.
// For local stubs (StubBinder), it auto-registers the stub with the transport
// (if not already registered) and writes BINDER_TYPE_BINDER with the cookie.
func WriteBinderToParcel(
	ctx context.Context,
	p *parcel.Parcel,
	b IBinder,
	transport Transport,
) {
	if b == nil {
		p.WriteNullStrongBinder()
		return
	}

	stub, isStub := b.(*StubBinder)
	if !isStub {
		p.WriteStrongBinder(b.Handle())
		return
	}

	cookie := stub.Cookie()
	if cookie == 0 {
		cookie = stub.RegisterWithTransport(ctx, transport)
	}
	p.WriteLocalBinder(stub.BinderPtr(), cookie)
}
