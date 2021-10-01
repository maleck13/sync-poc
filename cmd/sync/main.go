package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	// setup a dynamic client for handling unstructured content
	c, err := dynamic.NewForConfig(config)
	// setup a typed client for things we know about e.g namespaces
	tc, err := client.New(config, client.Options{})
	if err != nil {
		panic(err)
	}

	Reconcile(c, tc)
	b := make(chan struct{})
	<-b

}

// example payload that will come from the control plane
type Payload struct {
	Kind  string `json:"kind"`
	Page  string `json:"page"`
	Size  string `json:"size"`
	Total string `json:"total"`
	// leverge the unstructured type from kubernetes as it gives us the meta data we need without needing to care about specifics of the spec/status
	Items []*unstructured.Unstructured `json:"items"`
}

func groupVersionResourceFromUnstructured(o *unstructured.Unstructured) schema.GroupVersionResource {
	//TODO need to figure out if there is an API for getting the resource type
	// from breif reading it seems all resource names (the path used in the URL use the plural of the kind e.g "pods,namespaces,secrets...")
	resource := strings.ToLower(o.GetObjectKind().GroupVersionKind().Kind + "s")
	return schema.GroupVersionResource{Group: o.GetObjectKind().GroupVersionKind().Group, Version: o.GetObjectKind().GroupVersionKind().Version, Resource: resource}
}

func getControlPlanePayload() (*Payload, error) {
	resp, err := http.Get("http://localhost:8100/api/crontabs")
	if err != nil {
		return nil, fmt.Errorf("error getting control plane payload to sync %s", err.Error())
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error getting control plane payload to sync %s", err.Error())
	}

	payload := &Payload{}
	if err := json.Unmarshal(data, payload); err != nil {
		return nil, fmt.Errorf("error unmarshalling payload %s", err.Error())
	}
	return payload, nil
}

//stores our watches, but in reality we would do this via the operator SDK this is just a POC
var watches map[schema.GroupVersionResource]watch.Interface = make(map[schema.GroupVersionResource]watch.Interface)

func ensureNamespace(ctx context.Context, ns string, tc client.Client) error {
	n := &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	if err := tc.Create(ctx, n, &client.CreateOptions{}); err != nil {
		if !k8serr.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create namespace "+n.Name, err)
		}
	}
	return nil
}

func setupWatcher(ctx context.Context, o *unstructured.Unstructured, dc dynamic.Interface) error {
	gvr := groupVersionResourceFromUnstructured(o)
	annotations := o.GetAnnotations()
	if _, ok := annotations["status"]; !ok {
		// not status annotation nothing to do
		return nil
	}
	statusPaths := annotations["status"]
	fmt.Println("starting watch on ", gvr)
	if _, ok := watches[gvr]; ok {
		//already have a watch set up for this resource
		return nil
	}
	wi, err := dc.Resource(gvr).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error watching resource ", gvr)
	}
	go func() {
		for {
			select {
			case re, ok := <-wi.ResultChan():
				if !ok {
					delete(watches, gvr)
					return
				}
				rawObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(re.Object)
				if err != nil {
					fmt.Println("watch failed to convert object ", err)
				}
				individualPaths := strings.Split(statusPaths, ",")
				statusBody := map[string]interface{}{}
				for _, p := range individualPaths {
					jsonPath := strings.Split(p, ".")
					status, _, err := unstructured.NestedFieldCopy(rawObj, jsonPath...)
					if err != nil {
						fmt.Println("watch error getting status field value", err)
						continue
					}
					statusBody[p] = status
				}
				data, err := json.Marshal(statusBody)
				if err != nil {
					fmt.Println("error marshalling status", err)
					continue
				}
				b := bytes.NewBuffer(data)
				if _, err := http.Post("http://localhost:8100/api/status", "application/json", b); err != nil {
					fmt.Println("failed to post status", err)
					continue
				}
			}
		}
	}()
	// add our watch to the map so we don't add any new ones
	watches[gvr] = wi
	return nil
}

func Reconcile(c dynamic.Interface, tc client.Client) {

	t := time.Tick(5 * time.Second)
	ctx := context.TODO()

	for {
		select {
		case <-t:
			payload, err := getControlPlanePayload()
			if err != nil {
				fmt.Println(err)
				break
			}
			for _, o := range payload.Items {

				gvr := groupVersionResourceFromUnstructured(o)
				// if a namespace is set ensure the namespace exists
				if o.GetNamespace() != "" {
					if err := ensureNamespace(ctx, o.GetNamespace(), tc); err != nil {
						fmt.Println(err)
						continue
					}
				}
				// sync will trigger the delete but up to actual owners of the CRs to manager finalizers and owner refs
				if o.GetDeletionTimestamp() != nil {
					fmt.Println("deleting resource ", o.GetName())
					err := c.Resource(gvr).Namespace(o.GetNamespace()).Delete(ctx, o.GetName(), metav1.DeleteOptions{})
					if err != nil {
						fmt.Println("error deleting resource ", err.Error())
						continue
					}
					continue
				}
				// create update
				fmt.Println("createUpdate resource ", o.GetName())
				if _, err := c.Resource(gvr).Namespace(o.GetNamespace()).Create(ctx, o, metav1.CreateOptions{}); err != nil {
					if k8serr.IsAlreadyExists(err) {
						fmt.Println("resource "+o.GetName()+" exists updating ", err.Error())
						// get the existing resource and update it
						ob, err := c.Resource(gvr).Namespace(o.GetNamespace()).Get(ctx, o.GetName(), metav1.GetOptions{})
						if err != nil {
							fmt.Println("failed to get current object", err)
							continue
						}
						//update the resource version
						o.SetResourceVersion(ob.GetResourceVersion())
						if _, err := c.Resource(gvr).Namespace(o.GetNamespace()).Update(ctx, o, metav1.UpdateOptions{}); err != nil {
							fmt.Println("failed to update object"+o.GetName(), err)
							continue
						}
					}
				}
				// add a watch if we see a status annotation
				setupWatcher(ctx, o, c)
			}
		}
	}
}
