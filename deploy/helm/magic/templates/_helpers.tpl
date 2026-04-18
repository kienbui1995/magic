{{/*
Expand the name of the chart.
*/}}
{{- define "magic.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars (DNS label limit).
*/}}
{{- define "magic.fullname" -}}
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
Chart label (chart+version).
*/}}
{{- define "magic.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "magic.labels" -}}
helm.sh/chart: {{ include "magic.chart" . }}
{{ include "magic.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: magic
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "magic.selectorLabels" -}}
app.kubernetes.io/name: {{ include "magic.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service account name.
*/}}
{{- define "magic.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "magic.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Fully qualified image reference.
*/}}
{{- define "magic.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end }}

{{/*
Secret name — either user-provided or auto-generated.
*/}}
{{- define "magic.secretName" -}}
{{- if .Values.secrets.existingSecret }}
{{- .Values.secrets.existingSecret }}
{{- else }}
{{- include "magic.fullname" . }}
{{- end }}
{{- end }}

{{/*
ConfigMap name.
*/}}
{{- define "magic.configMapName" -}}
{{- printf "%s-config" (include "magic.fullname" .) }}
{{- end }}
