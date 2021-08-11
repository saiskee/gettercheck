package In

import (
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

func main() {
	// todo(sai): resolve Name field error
	//_ = (&envoy_config_route_v3.Route{
	//	Name:                                 (&envoy_config_route_v3.Route{}).Name,
	//}).GetMetadata().FilterMetadata
	//a := envoy_config_route_v3.VirtualHost{}
	a := &descriptor.FileDescriptorSet{}
	for _, file := range a.GetFile() {
		if file.Package == nil {

		}
	}
	//a.GetCors().GetAllowCredentials().Value = true
	//_ = &a.Cors.AllowCredentials.Value
	//a.Value = 9
	//b := envoy_config_core_v3.HeaderValueOption{}
	//print(b.Header.Value)
	//fmt.Println(r.Name)
	//var (
	//	_ = r.ResponseHeadersToAdd[0].Header
	//	_ = r.ResponseHeadersToAdd
	//)
}
