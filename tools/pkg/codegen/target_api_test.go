package codegen

import (
	"strings"
	"testing"

	"github.com/AndroidGoLab/binder/tools/pkg/parser"
)

func TestGeneratedTargetAPIPropagation(t *testing.T) {
	doc := parseAIDL(t, `package example; interface IExample { int call(int input); oneway void notify(int input); }`)
	src, err := GenerateInterface(doc.Definitions[0].(*parser.InterfaceDecl), "example", "example.IExample")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, want := range []string{"_data.SetTargetAPILevel(binder.APILevelFromBinder(p.Remote))", "_reply.SetTargetAPILevel(_data.TargetAPILevel())"} {
		if !strings.Contains(s, want) {
			t.Errorf("generated code missing %q", want)
		}
	}
	if strings.Count(s, "_data.SetTargetAPILevel(binder.APILevelFromBinder(p.Remote))") != 2 {
		t.Error("both oneway and synchronous requests must carry the profile")
	}
	if strings.Count(s, "_data.SetBinderSerializationContext(binder.NewBinderSerializationContext(p.Remote.Transport()))") != 2 {
		t.Error("both requests must carry their transport context")
	}
	if strings.Count(s, "_reply.SetBinderSerializationContext(_data.BinderSerializationContext())") != 2 {
		t.Error("proxy and stub replies must carry the transport context")
	}
	if strings.Count(s, "_reply.SetDisplayInfoWireProfile(_data.DisplayInfoWireProfile())") != 2 {
		t.Error("proxy and stub replies must carry the DisplayInfo profile")
	}
	if !strings.Contains(s, "if _reply.DisplayInfoWireProfile() == parcel.DisplayInfoWireProfileUnknown {") {
		t.Error("proxy must preserve a profile supplied by the transport")
	}
	if !strings.Contains(s, "if _replyOwned {") || !strings.Contains(s, "_reply.Recycle()") || strings.Count(s, "_replyOwned = false") != 2 {
		t.Error("stub must recycle an untransferred reply and transfer successful replies")
	}
	if strings.Count(s, "_reply.SetTargetAPILevel(_data.TargetAPILevel())") != 2 {
		t.Error("proxy and stub replies must carry the profile")
	}
	if strings.Index(s, "_data.SetTargetAPILevel(binder.APILevelFromBinder(p.Remote))") > strings.Index(s, "_data.WriteInterfaceToken") {
		t.Error("profile must precede encoding")
	}
	stub := s[strings.Index(s, "func (s *ExampleStub) OnTransaction"):]
	proxy := s[strings.Index(s, "func (p *ExampleProxy) Call"):]
	if strings.Index(stub, "_reply.SetTargetAPILevel(_data.TargetAPILevel())") > strings.Index(stub, "binder.WriteStatus(_reply") {
		t.Error("stub profile must precede reply encoding")
	}
	if strings.Index(proxy, "_reply.SetTargetAPILevel(_data.TargetAPILevel())") > strings.Index(proxy, "binder.ReadStatus(_reply)") {
		t.Error("proxy reply profile must precede decoding")
	}
}
