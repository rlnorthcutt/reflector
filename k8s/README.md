# Kubernetes examples

Three replicas behind a Service, with a custom route mounted from a
ConfigMap and readiness wired to reflector's built-in probe.

```sh
kubectl apply -k k8s/
kubectl port-forward svc/reflector 8080:80
curl localhost:8080/api/status   # the ConfigMap-mounted route: "served_by" names the pod
curl localhost:8080/             # the built-in envelope: server.hostname (or "Hostname:") names the pod
```

Hit either route repeatedly and watch that pod-identifying field change to
see requests landing on different pods — this is the building block for
the load-balancing and failover demos in the main README.

- `configmap.yaml` — a `routes.d/*.yaml` file mounted at `/app/routes.d`.
  Edit it and `kubectl apply` again to change routes without rebuilding
  the image.
- `deployment.yaml` — 3 replicas; `imagePullPolicy: IfNotPresent` so a
  locally built and `kind load`-ed image works without a registry.
- `service.yaml` — a ClusterIP Service for traffic (port 80 → 8080), and
  a **separate** ClusterIP Service for the admin API (port 8081), kept
  off any LoadBalancer/NodePort on purpose since the admin API is meant
  to be firewalled away from regular traffic. Reach it with
  `kubectl port-forward svc/reflector-admin 8081:8081`.
