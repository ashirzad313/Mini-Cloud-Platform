package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// podInfo is the small part of Kubernetes' Pod API response we expose.
type podInfo struct {
	Name  string `json:"name"`
	Phase string `json:"phase"`
}

func registerClusterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /cluster/pods", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		namespace, pods, err := listClusterPods(ctx)
		if err != nil {
			log.Printf("list Kubernetes Pods: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "kubernetes_unavailable"})
			return
		}

		writeJSON(w, http.StatusOK, struct {
			Namespace string    `json:"namespace"`
			Pods      []podInfo `json:"pods"`
		}{
			Namespace: namespace,
			Pods:      pods,
		})
	})
}

// listClusterPods calls the real Kubernetes REST API from inside the cluster.
// Kubernetes automatically mounts a scoped ServiceAccount token and CA certificate.
func listClusterPods(ctx context.Context) (string, []podInfo, error) {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	if host == "" {
		return "", nil, fmt.Errorf("KUBERNETES_SERVICE_HOST is unset (API must run inside Kubernetes)")
	}
	port := os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
	if port == "" {
		port = "443"
	}

	namespaceBytes, err := os.ReadFile(serviceAccountDir + "/namespace")
	if err != nil {
		return "", nil, fmt.Errorf("read ServiceAccount namespace: %w", err)
	}
	namespace := strings.TrimSpace(string(namespaceBytes))

	tokenBytes, err := os.ReadFile(serviceAccountDir + "/token")
	if err != nil {
		return "", nil, fmt.Errorf("read ServiceAccount token: %w", err)
	}

	caBytes, err := os.ReadFile(serviceAccountDir + "/ca.crt")
	if err != nil {
		return "", nil, fmt.Errorf("read Kubernetes CA certificate: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caBytes) {
		return "", nil, fmt.Errorf("invalid Kubernetes CA certificate")
	}

	kubernetesURL := &url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(host, port),
		Path:   "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods",
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, kubernetesURL.String(), nil)
	if err != nil {
		return "", nil, fmt.Errorf("prepare Kubernetes request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tokenBytes)))

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs:    roots,
			MinVersion: tls.VersionTLS12,
		}},
	}
	response, err := client.Do(request)
	if err != nil {
		return "", nil, fmt.Errorf("call Kubernetes API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("Kubernetes API returned %s", response.Status)
	}

	var result struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", nil, fmt.Errorf("decode Kubernetes Pod list: %w", err)
	}

	pods := make([]podInfo, 0, len(result.Items))
	for _, item := range result.Items {
		pods = append(pods, podInfo{Name: item.Metadata.Name, Phase: item.Status.Phase})
	}
	return namespace, pods, nil
}
