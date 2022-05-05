IMAGE:=databus23/sml-exporter:0.1.0

build:
	docker build --platform=linux/arm64 -t $(IMAGE) .

push:
	docker push $(IMAGE)
