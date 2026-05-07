# Security Policy

## Supported versions

ChromaFlow is pre-1.0. Security fixes are expected to land on the default branch and in the latest tagged release.

## Reporting vulnerabilities

Please report suspected vulnerabilities privately through the repository owner's preferred security channel. If GitHub private vulnerability reporting is enabled, use that feature. Otherwise, open a minimal issue requesting a private contact path without disclosing exploit details.

## Deployment guidance

ChromaFlow renders attacker-provided URLs in Chromium. Before exposing it outside a trusted network, deployers should add or enforce:

- Authentication and authorization.
- Rate limits and request body limits at the edge.
- SSRF protections for private IP ranges, loopback, link-local addresses, metadata endpoints, DNS rebinding, and redirects.
- Network egress controls around the service and Chromium.
- CPU and memory limits sized for Chromium workloads.
- Monitoring for queue depth, render failures, and resource exhaustion.
