package services


type AccountbalanceService interface {
	GetBalance(address, token, network string) ([]map[string]string, error)
}
