package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

func main() {

	r := mux.NewRouter()

	r.HandleFunc("/api/crontabs", Crontabs())

	srv := &http.Server{
		Handler:      r,
		Addr:         "127.0.0.1:8100",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}

var apiResp = `
{
  "kind": "",
  "page": "1",
  "size": "3",
  "total": "3",
  "items":[
	{
		"apiVersion": "stable.example.com/v1",
		"kind": "CronTab",
		"metadata": {
			"name": "my-new-cron-object",
			"namespace":"test",
			"annotations":{
				"status":"spec"
			}
		},
		"spec": {
			"cronSpec": "* * * * */10",
			"image": "my-awesome-cron-image"
		}
	},
	{
		"apiVersion": "stable.example.com/v1",
		"kind": "CronTab",
		"metadata": {
			"deletionTimestamp": "2018-08-24T17:15:39Z",
			"name": "my-old-cron-object",
			"namespace":"test"
		},
		"spec": {
			"cronSpec": "* * * * */5",
			"image": "my-awesome-cron-image",
			"secretRef":"mysecret"
		}
	},
	{
		"apiVersion": "v1",
		"data": {
			"password": "cGFzc3dvcmQ=",
			"username": "dXNlci1uYW1l"
		},
		"kind": "Secret",
		"metadata": {
			"name": "mysecret",
			"namespace": "test"
		},
		"type": "Opaque"
	}
  ]
}
`

func Crontabs() func(rw http.ResponseWriter, req *http.Request) {

	return func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Add("content-type", "application/json")
		rw.Write([]byte(apiResp))
	}

}
