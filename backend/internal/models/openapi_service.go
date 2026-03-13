package models

import "time"

type OpenAPIServiceRequest struct {
	Name                     string               `json:"name" binding:"required"`
	Type                     string               `json:"type,omitempty"`
	TargetPodName            string               `json:"targetPodName,omitempty"`
	Selector                 map[string]string    `json:"selector,omitempty"`
	Ports                    []OpenAPIServicePort `json:"ports" binding:"required,min=1"`
	SessionAffinity          string               `json:"sessionAffinity,omitempty"`
	PublishNotReadyAddresses bool                 `json:"publishNotReadyAddresses,omitempty"`
	Annotations              map[string]string    `json:"annotations,omitempty"`
}

type OpenAPIServicePort struct {
	Name       string `json:"name,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	Port       int32  `json:"port" binding:"required,min=1,max=65535"`
	TargetPort string `json:"targetPort,omitempty"`
	NodePort   int32  `json:"nodePort,omitempty"`
}

type OpenAPIServiceResponse struct {
	Name         string               `json:"name"`
	Namespace    string               `json:"namespace"`
	Type         string               `json:"type"`
	Selector     map[string]string    `json:"selector,omitempty"`
	Ports        []OpenAPIServicePort `json:"ports"`
	ClusterIP    string               `json:"clusterIP,omitempty"`
	ExternalIPs  []string             `json:"externalIPs,omitempty"`
	LoadBalancer []string             `json:"loadBalancer,omitempty"`
	CreatedAt    time.Time            `json:"createdAt"`
}

type OpenAPIServiceListResponse struct {
	Services []OpenAPIServiceResponse `json:"services"`
}
