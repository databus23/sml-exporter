sml-exporter
===========

sml-exporter is exporter for sml enabled smartmeters.
It currenlty support exposing sml data in the following ways:

* prometheus metrics
* mqtt 



Usage
=====

```
Usage of sml-exporter:
  -config string
    	configfile with obis code mappings
  -debug
    	Enable debug logging
  -metrics-address string
    	The address to listen on for HTTP requests. (default ":9761")
  -mqtt-password string
    	MQTT password
  -mqtt-server string
    	MQTT server to publish values to as they are received
  -mqtt-topic-prefix string
    	MQTT topic prefix for publishing values (default "smartmeter")
  -mqtt-username string
    	MQTT username
  -serial string
    	Serial device to read fro
```

## Obis code mapping

How individual obis codes are exposed is configurable via a simple configuration file

Example:
```
1-0:1.8.0*255:
  mqtt: 
    topic: "smartmeter/wirkarbeit-verbrauch"
  metric:
    name: "smartmeter_wirkarbeit_verbrauch_wh_total"
1-0:2.8.0*255:
  mqtt: 
    topic: "smartmeter/wirkarbeit-einspeisung"
  metric:
    name: "smartmeter_wirkarbeit_einspeisung_wh_total"
1-0:16.7.0*255:
  mqtt: 
    topic: "smartmeter/momentane-wirkleistung"
  metric:
    name: "smartmeter_leistung_gesamt_w"
```

