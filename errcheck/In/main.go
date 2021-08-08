package In

import envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

func main(){
	// todo(sai): resolve Name field error
	//_ = (&envoy_config_route_v3.Route{
	//	Name:                                 (&envoy_config_route_v3.Route{}).Name,
	//}).GetMetadata().FilterMetadata
	_ = envoy_config_route_v3.WeightedCluster{}.TotalWeight.Value
	//a.Value = 9
	//b := envoy_config_core_v3.HeaderValueOption{}
	//print(b.Header.Value)
	//fmt.Println(r.Name)
	//var (
	//	_ = r.ResponseHeadersToAdd[0].Header
	//	_ = r.ResponseHeadersToAdd
	//)
}