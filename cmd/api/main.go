package main

import (
	"flag"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {

	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	r := mux.NewRouter()

	r.HandleFunc("/api/crontabs", Crontabs(*clientset))

	srv := &http.Server{
		Handler:      r,
		Addr:         "127.0.0.1:8100",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}

/// apis/stable.example.com/v1/namespaces/*/crontabs/...
var apiResp = `
{
  "kind": "",
  "page": "1",
  "size": "2",
  "total": "2",
  "items":[
	{
		"apiVersion": "stable.example.com/v1",
		"kind": "CronTab",
		"metadata": {
			"name": "my-new-cron-object",
			"namespace":"test"
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
			"image": "my-awesome-cron-image"
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

func Crontabs(cs kubernetes.Clientset) func(rw http.ResponseWriter, req *http.Request) {

	return func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Add("content-type", "application/json")
		rw.Header().Add("X-FM-GVK", "stable.example.com,v1,crontabs")
		rw.Header().Add("X-FM-RESOURCE-NAMES", "my-new-cron-object,my-old-cron-object")
		rw.Header().Add("X-FM-DELETE", "my-old-cron-object")
		rw.Write([]byte(apiResp))

	}

}
