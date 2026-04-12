# sml-exporter

sml-exporter is an exporter for SML-enabled smart meters.
It currently supports exposing SML data in the following ways:

* Prometheus metrics
* MQTT

## Usage

```
Usage of sml-exporter:
  -config string
    	Config file with OBIS code mappings
  -debug
    	Enable debug logging
  -health-timeout duration
    	Timeout duration for health check (default 10s)
  -metrics-address string
    	The address to listen on for HTTP requests (default ":9761")
  -mqtt-password string
    	MQTT password
  -mqtt-server string
    	MQTT server to publish values to as they are received
  -mqtt-topic-prefix string
    	MQTT topic prefix for publishing values (default "smartmeter")
  -mqtt-username string
    	MQTT username
  -serial string
    	Serial device to read from
```

A health check endpoint is available at `/healthz`. It returns 200 if data has been received within the configured `-health-timeout`, and 503 otherwise.

## OBIS code mapping

How individual OBIS codes are exposed is configurable via a YAML configuration file (see `example-config.yaml` for a full example).

Each entry maps an OBIS code to an optional MQTT topic, Prometheus metric, or named variable:

```yaml
1-0:96.1.0*255:
  type: string
  var: server_id
1-0:1.8.0*255:
  mqtt:
    topic: "smartmeter/wirkarbeit-verbrauch"
  metric:
    name: "smartmeter_wirkarbeit_verbrauch_wh_total"
    help: "Total active energy consumed in Wh"
1-0:2.8.0*255:
  mqtt:
    topic: "smartmeter/wirkarbeit-einspeisung"
  metric:
    name: "smartmeter_wirkarbeit_einspeisung_wh_total"
    help: "Total active energy fed back in Wh"
1-0:16.7.0*255:
  mqtt:
    topic: "smartmeter/momentane-wirkleistung"
  metric:
    name: "smartmeter_leistung_gesamt_w"
    help: "Current total active power in W"
```

Entries with `type: string` and `var` store the value as a named variable instead of exporting it. These variables can be referenced elsewhere — for example, `server_id` is automatically added as a label to all Prometheus metrics, identifying which smart meter the readings came from.

## Docker

Pre-built multi-arch Docker images (linux/amd64, linux/arm64) are published to Docker Hub:

```
docker pull databus23/sml-exporter
```

```bash
docker run --device /dev/ttyUSB0 -v ./config.yaml:/etc/sml-exporter/config.yaml \
  databus23/sml-exporter -serial /dev/ttyUSB0 -config /etc/sml-exporter/config.yaml
```

## Kubernetes

Example Kubernetes manifests are provided in [`examples/kubernetes/`](examples/kubernetes/). The example uses Kustomize to deploy the exporter with a ConfigMap-based configuration:

```bash
# Review and adjust examples/kubernetes/config.yaml for your meter
kubectl apply -k examples/kubernetes/
```

The deployment requires `privileged: true` to access the serial device on the host. Adjust the serial device path in the deployment if your device is not at `/dev/ttyUSB0`.


