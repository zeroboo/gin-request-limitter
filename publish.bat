SET VERSION=v0.2.0

git tag %VERSION%
git push origin %VERSION%
SET GOPROXY=proxy.golang.org 
go list -m github.com/zeroboo/gin-request-limitter@%VERSION%
