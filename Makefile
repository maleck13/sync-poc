.PHONY: setup

setup:
	go build ./cmd/sync
	go build ./cmd/api
	-oc create -f crd.yaml	
	-oc create ns test
	-oc create -f old.yaml -n test