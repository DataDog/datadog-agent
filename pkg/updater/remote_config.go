package rc

import (
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
)

type UpdaterService struct {
	service *service.Service
	client  *client.Client
}

func NewUpdaterService() (*UpdaterService, error) {

}

func (s *UpdaterService) GetOrgConfig() ([][]byte, error) {

}
