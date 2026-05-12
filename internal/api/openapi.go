package api

var openAPISpec = []byte(`openapi: 3.0.3
info:
  title: ChromaFlow API
  version: 0.1.0
  description: API for submitting webpage-to-PDF jobs, polling job status, downloading completed PDFs, and operating ChromaFlow.
servers:
  - url: http://localhost:8080
paths:
  /:
    get:
      summary: Dashboard
      responses:
        '200':
          description: HTML dashboard.
          content:
            text/html:
              schema:
                type: string
  /pdf:
    post:
      summary: Submit a PDF job
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/SubmitRequest'
      responses:
        '202':
          description: Job accepted.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SubmitResponse'
        '400':
          description: Invalid JSON or URL.
          content:
            text/plain:
              schema:
                type: string
        '503':
          description: In-memory queue is full.
          content:
            text/plain:
              schema:
                type: string
  /pdf/{id}:
    get:
      summary: Fetch job status or completed PDF
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        '200':
          description: Job status JSON for non-completed jobs or PDF bytes for completed jobs.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/JobStatus'
            application/pdf:
              schema:
                type: string
                format: binary
        '404':
          description: Job not found.
          content:
            text/plain:
              schema:
                type: string
  /ws/jobs:
    get:
      summary: Job snapshot websocket
      description: Upgrades to a websocket and sends full job snapshot messages whenever jobs change.
      responses:
        '101':
          description: Websocket upgrade accepted.
        '400':
          description: Websocket upgrade failed.
  /healthz:
    get:
      summary: Liveness probe
      responses:
        '200':
          description: Service is alive.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/HealthStatus'
  /readyz:
    get:
      summary: Readiness probe
      responses:
        '200':
          description: Service is ready to accept traffic.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReadyStatus'
  /version:
    get:
      summary: Build version
      responses:
        '200':
          description: Build version injected at compile time.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Version'
  /metrics:
    get:
      summary: Prometheus metrics
      responses:
        '200':
          description: Metrics in Prometheus text exposition format.
          content:
            text/plain:
              schema:
                type: string
  /openapi.yaml:
    get:
      summary: OpenAPI document
      responses:
        '200':
          description: OpenAPI YAML document.
          content:
            application/yaml:
              schema:
                type: string
components:
  schemas:
    SubmitRequest:
      type: object
      required: [url]
      properties:
        url:
          type: string
          format: uri
          example: https://example.com
    SubmitResponse:
      type: object
      required: [job_id, status_url]
      properties:
        job_id:
          type: string
          format: uuid
        status_url:
          type: string
          example: /pdf/00000000-0000-0000-0000-000000000000
    JobStatus:
      type: object
      required: [job_id, url, status, error]
      properties:
        job_id:
          type: string
          format: uuid
        url:
          type: string
          format: uri
        status:
          type: string
          enum: [pending, processing, completed, failed]
        error:
          type: string
    JobSnapshotMessage:
      type: object
      required: [type, jobs]
      properties:
        type:
          type: string
          enum: [jobs]
        jobs:
          type: array
          items:
            $ref: '#/components/schemas/JobSnapshot'
    JobSnapshot:
      type: object
      required: [id, url, status, created_at, updated_at]
      properties:
        id:
          type: string
          format: uuid
        url:
          type: string
          format: uri
        status:
          type: string
          enum: [pending, processing, completed, failed]
        error:
          type: string
        created_at:
          type: string
          format: date-time
        updated_at:
          type: string
          format: date-time
    HealthStatus:
      type: object
      properties:
        status:
          type: string
          example: ok
    ReadyStatus:
      type: object
      properties:
        status:
          type: string
          example: ready
    Version:
      type: object
      properties:
        version:
          type: string
          example: v0.1.0
`)
