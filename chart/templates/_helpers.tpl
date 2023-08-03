{{/*
Expand the name of the chart.
*/}}
{{- define "name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}


{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "chartName" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
This is the version the chart is intended to be used with.
*/}}
{{- define "appVersion" -}}
{{- .Chart.AppVersion -}}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "selectorLabels" -}}
app.kubernetes.io/name: {{ include "name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "commonLabels" -}}
helm.sh/chart: {{ include "chartName" . }}
{{ include "selectorLabels" . }}
{{- if (include "appVersion" .) }}
app.kubernetes.io/version: {{ (include "appVersion" .) | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/created-by: kube-shuffle-sharder
app.kubernetes.io/part-of: kube-shuffle-sharder
{{- end }}

