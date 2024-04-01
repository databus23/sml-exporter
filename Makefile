IMAGE:=databus23/sml-exporter:0.6.3

build:
	docker build --platform=linux/arm64 -t $(IMAGE) .

push:
	docker push $(IMAGE)
