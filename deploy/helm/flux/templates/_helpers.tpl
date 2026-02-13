{{/*
Expand the name of the chart.
*/}}
{{- define "flux.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "flux.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "flux.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "flux.labels" -}}
helm.sh/chart: {{ include "flux.chart" . }}
{{ include "flux.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "flux.selectorLabels" -}}
app.kubernetes.io/name: {{ include "flux.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Database URL
*/}}
{{- define "flux.databaseURL" -}}
{{- if .Values.postgresql.enabled -}}
postgres://{{ .Values.postgresql.auth.username }}:{{ .Values.postgresql.auth.password }}@{{ include "flux.fullname" . }}-postgresql:5432/{{ .Values.postgresql.auth.database }}?sslmode=disable
{{- else -}}
postgres://{{ .Values.externalDatabase.user }}:{{ .Values.externalDatabase.password }}@{{ .Values.externalDatabase.host }}:{{ .Values.externalDatabase.port }}/{{ .Values.externalDatabase.database }}?sslmode=disable
{{- end -}}
{{- end }}

{{/*
NATS URL
*/}}
{{- define "flux.natsURL" -}}
{{- if .Values.nats.enabled -}}
nats://{{ include "flux.fullname" . }}-nats:4222
{{- else -}}
{{ .Values.externalNats.url }}
{{- end -}}
{{- end }}

{{/*
Redis URL
*/}}
{{- define "flux.redisURL" -}}
{{- if .Values.redis.enabled -}}
redis://{{ include "flux.fullname" . }}-redis-master:6379/0
{{- else -}}
{{ .Values.externalRedis.url }}
{{- end -}}
{{- end }}
