{{- define "nexuschat.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "nexuschat.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- include "nexuschat.name" . -}}
{{- end -}}
{{- end -}}

{{- define "nexuschat.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "nexuschat.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: nexuschat
{{- end -}}

{{- define "nexuschat.serviceLabels" -}}
{{ include "nexuschat.labels" .root }}
app.kubernetes.io/component: {{ .name }}
{{- end -}}

{{- define "nexuschat.image" -}}
{{- $registry := trimSuffix "/" .root.Values.global.imageRegistry -}}
{{- $repository := .svc.image.repository -}}
{{- $tag := default .root.Values.imageDefaults.tag .svc.image.tag -}}
{{- printf "%s/%s:%s" $registry $repository $tag -}}
{{- end -}}
