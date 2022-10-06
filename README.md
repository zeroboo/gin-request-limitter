# gin-request-limitter
Request limitter as middleware for gin-gonic framework: limit requests can be accepted of an user.


# Features:
  - Limit request interval: user can not send request too fast
  - Limit request freequently:
    User can not send too many requests in a time window. 
    Implement Fixed windows algorithm using Redis or Google Firestore as persistence.
# Usage
Install
```console
go get github.com/zeroboo/gin-request-limitter
```
Run:
```go

	dsClient, _ := datastore.NewClient(context.Background(), datastoreProjectId, option.WithCredentialsFile(serviceAccount))
  
  handler:=CreateDatastoreBackedLimitter(dsClient, TEST_REQUEST_TRACKER_DATASTORE_KIND, GetUserIdFromContextByField("userId"), 200),
  
  //use handler ...

```

Publish:

SET GOPROXY=proxy.golang.org 
go list -m github.com/zeroboo/gin-request-limitter@v0.3.0
