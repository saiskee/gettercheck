package In

import (
	//"fmt"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	//envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

)

func main(){
	_ = envoy_config_route_v3.Route{
		Name:                                 envoy_config_route_v3.Route{}.Name,
	}
	//b := envoy_config_core_v3.HeaderValueOption{}
	//print(b.Header.Value)
	//fmt.Println(r.Name)
	//var (
	//	_ = r.ResponseHeadersToAdd[0].Header
	//	_ = r.ResponseHeadersToAdd
	//)
}