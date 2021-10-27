# Generic Sync

This POC code that illustrates how it is possible to sync resources from an API to a target kubernetes cluster without having to know the specific types. It also shows a mechanism for specifying data that you want reported back.


## API

There is a basic API server that sends the payload to the sync client. It is very simplistic and just serves a static set of resources

## Sync

The sync component is also very simple and can be thought of as a dumb pipe. In reality it would probably be made into a controller. Currently it polls the API server once every 5 seconds and then process the payload.

It attempt to delete any resource it sees with a metadata deletionTimestamp.

It will attempt to create any resource without this deletionTimestamp. It will also try to create any namespace specified by the resource. If the resource exists it will attempt to update the resource. 

If a resource has a status annotation, it will setup a watch on that resource. The status annoation specifies a json path to the pieces of the resource that the control plane wants reported back. Example `"status":"status"` as an annotation will result in a watch set up for that resource type.
It may be neccessary for the watch to limit itself to the set of namespaces it knows about.  Using the unstructured helpers the watch can pull out fields from the resources without needing to know the concrete type. It could then send this payload back to the fleet manager
