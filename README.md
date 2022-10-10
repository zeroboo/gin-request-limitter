# gin-request-limitter
Request limitter as middleware for gin-gonic framework: limit requests can be accepted of an user.


# Features:
  - Limit request interval: user can not send request too fast
  - Limit request freequently:
    User can not send too many requests in a time window. 
    Implement Fixed windows algorithm using Redis or Google Firestore as persistence.
# Usage
* Install
```console
go get github.com/zeroboo/gin-request-limitter
```
* Run:
```go

ctx := context.Background()
dsClient, _ := datastore.NewClient(ctx, datastoreProjectId, option.WithCredentialsFile(serviceAccount))
  
/*
Create a limitter middleware per user:
  persists data to kind 'tracker' in datastore
  collect userId from gin context by field 'userId'
  restricts request only send to 200 milis after the last one
  restricts maximum 100 requests per 60 seconds
*/
handler := CreateDatastoreBackedLimitter(dsClient,
  "tracker",
  GetUserIdFromContextByField("userId"), 
  200, 
  60000, 
  100),
  
//use handler ...
```

* Test: 
```console
go test -timeout 30s github.com/zeroboo/gin-request-limitter -v
```
* Publish:  

```console
SET GOPROXY=proxy.golang.org 
go list -m github.com/zeroboo/gin-request-limitter@[VERSION]
```
