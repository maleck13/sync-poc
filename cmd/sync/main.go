package main

import (
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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	c, err := dynamic.NewForConfig(config)

	tc, err := client.New(config, client.Options{})
	if err != nil {
		panic(err)
	}

	Reconcile(c, tc)
	b := make(chan struct{})
	<-b

}

type syncPayload struct {
	Kind  string                       `json:"kind"`
	Page  string                       `json:"page"`
	Size  string                       `json:"size"`
	Total string                       `json:"total"`
	Items []*unstructured.Unstructured `json:"items"`
}

func groupVersionResource(o *unstructured.Unstructured) schema.GroupVersionResource {
	//TODO need to figure out if there is an API for getting the resource type
	return schema.GroupVersionResource{Group: o.GetObjectKind().GroupVersionKind().Group, Version: o.GetObjectKind().GroupVersionKind().Version, Resource: strings.ToLower(o.GetObjectKind().GroupVersionKind().Kind + "s")}
}

// look at status

func sync() (*syncPayload, error) {
	resp, err := http.Get("http://localhost:8100/api/crontabs")
	if err != nil {
		return nil, fmt.Errorf("error getting stuff to sync %s", err.Error())
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error getting stuff to sync %s", err.Error())
	}

	payload := &syncPayload{}
	if err := json.Unmarshal(data, payload); err != nil {
		return nil, fmt.Errorf("error decerning sync payload %s", err.Error())
	}
	return payload, nil
}

func Reconcile(c dynamic.Interface, tc client.Client) {

	t := time.Tick(5 * time.Second)
	ctx := context.TODO()
	for {
		select {
		case <-t:
			payload, err := sync()
			if err != nil {
				fmt.Println(err)
				break
			}
			for _, o := range payload.Items {
				t, _ := meta.Accessor(o)
				gvr := groupVersionResource(o)
				ns := &v1.Namespace{
					TypeMeta: metav1.TypeMeta{
						Kind:       "namespace",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: o.GetNamespace(),
					},
				}
				if err := tc.Create(ctx, ns, &client.CreateOptions{}); err != nil {
					if !k8serr.IsAlreadyExists(err) {
						fmt.Print("failed to create namespace "+ns.Name, err)
						continue
					}
				}
				if t.GetDeletionTimestamp() != nil {
					fmt.Println("deleting resource ", t.GetName())
					err := c.Resource(gvr).Namespace(t.GetNamespace()).Delete(ctx, t.GetName(), metav1.DeleteOptions{})
					if err != nil {
						fmt.Println("error deleting resource ", err.Error())
						continue
					}
					fmt.Println("deleted resource ", t.GetName())
					continue
				}
				// create update
				if _, err := c.Resource(gvr).Namespace(t.GetNamespace()).Create(ctx, o, metav1.CreateOptions{}); err != nil {
					fmt.Println("create failed ", err.Error())
					if k8serr.IsAlreadyExists(err) {
						fmt.Println("resource exists updating ", err.Error())
						ob, err := c.Resource(gvr).Namespace(t.GetNamespace()).Get(ctx, t.GetName(), metav1.GetOptions{})
						if err != nil {
							fmt.Println("failed to get current object", err)
							continue
						}
						//update the resource version
						t.SetResourceVersion(ob.GetResourceVersion())
						if _, err := c.Resource(gvr).Namespace(t.GetNamespace()).Update(ctx, o, metav1.UpdateOptions{}); err != nil {
							fmt.Println("failed to update object", err)
							continue
						}
						fmt.Println("updated resource ", o.GetName())
					}
				}
			}

		}
	}
}
