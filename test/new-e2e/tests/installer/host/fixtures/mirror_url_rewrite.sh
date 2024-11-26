#!/bin/bash
while read -r url; do
    if [[ $url =~ ^.*(/mirror/)(.*)$ ]]; then
        # Redirect /mirror/<path> to install.datadoghq.com/<path>
        path=${BASH_REMATCH[2]}
        echo "https://install.datadoghq.com/$path"
    else
        # Allow other URLs (e.g., the ECR domain) without rewriting
        echo "$url"
    fi
done
