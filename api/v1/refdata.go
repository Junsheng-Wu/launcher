package v1

type DataRef struct {
	NameSpace string `json:"namespace"` // todo 小写
	Name      string `json:"name"`
}

func (data *DataRef) IsEmpty() bool {
	if data == nil || len(data.Name) == 0 {
		return true
	}
	return false
}

type SecretRef = DataRef
