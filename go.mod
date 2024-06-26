module github.com/databus23/sml-exporter

go 1.17

require (
	github.com/eclipse/paho.mqtt.golang v1.4.2
	github.com/jacobsa/go-serial v0.0.0-20180131005756-15cf729a72d4
	github.com/mfmayer/gosml v0.0.1
	github.com/prometheus/client_golang v1.12.1
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	golang.org/x/net v0.0.0-20210525063256-abc453219eb5 // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20220114195835-da31bd327af9 // indirect
	google.golang.org/protobuf v1.26.0 // indirect
)

replace github.com/mfmayer/gosml => github.com/databus23/gosml v0.0.0-20240225223655-5876bb6ff120
