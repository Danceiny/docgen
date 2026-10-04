package engine

type ServiceInterface struct {
	ServiceName string // the value that the Name() string method returns; it makes the API path: <prefix>/{serviceName}/{methodName}
	Methods     []*Method
}
