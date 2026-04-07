IMAGE:=databus23/sml-exporter:latest

build:
	docker build --platform=linux/arm64 -t $(IMAGE) .

push:
	docker push $(IMAGE)
