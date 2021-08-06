package In

import (
	"fmt"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

func main(){
	r := envoy_config_route_v3.Route{}
	fmt.Println(r.Name)
	var (
		_ = r.ResponseHeadersToAdd[0].Header
		_ = r.ResponseHeadersToAdd
	)
}