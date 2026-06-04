#!/usr/bin/env bash
# remarkable-sync.sh
# Called by the remarkable-sync s6 service on each sync cycle.
# Assumes build-env.sh has already been sourced and RMAPI_CONFIG is set.
set -euo pipefail

RM_FOLDER="${REMARKABLE_RM_FOLDER:-/}"
OBS_FOLDER="${VAULT_PATH}/${REMARKABLE_OBSIDIAN_FOLDER:-reMarkable}"
BIDIRECTIONAL="${REMARKABLE_BIDIRECTIONAL:-true}"

UPLOAD_DIR="${OBS_FOLDER}/Upload"
UPLOADED_DIR="${OBS_FOLDER}/Uploaded"

mkdir -p "${OBS_FOLDER}" "${UPLOAD_DIR}" "${UPLOADED_DIR}"

# ---------------------------------------------------------------------------
# download_rm_folder <rm_path> <local_dir>
# Recursively downloads all documents from a reMarkable folder as PDFs.
# Skips files that already exist locally (re-download by deleting the local PDF).
# ---------------------------------------------------------------------------
download_rm_folder() {
    local rm_path="$1"
    local local_dir="$2"

    mkdir -p "${local_dir}"

    # rmapi ls output format:  [d]\tName  or  [f]\tName
    while IFS=$'\t' read -r type_tag name; do
        # Strip surrounding whitespace and brackets from type tag
        local item_type
        item_type=$(echo "${type_tag}" | tr -d '[] ')
        local item_name
        item_name=$(echo "${name}" | sed 's/^[[:space:]]*//')

        [ -z "${item_name}" ] && continue

        if [ "${item_type}" = "d" ]; then
            download_rm_folder "${rm_path}/${item_name}" "${local_dir}/${item_name}"
        elif [ "${item_type}" = "f" ]; then
            local pdf_dest="${local_dir}/${item_name}.pdf"
            if [ ! -f "${pdf_dest}" ]; then
                bashio::log.info "reMarkable → Obsidian: ${rm_path}/${item_name}"
                (cd "${local_dir}" && rmapi get "${rm_path}/${item_name}") 2>&1 | \
                    while IFS= read -r line; do bashio::log.debug "${line}"; done || \
                    bashio::log.warning "Failed to download: ${rm_path}/${item_name}"
            fi
        fi
    done < <(rmapi ls "${rm_path}" 2>/dev/null || true)
}

# ---------------------------------------------------------------------------
# upload_to_rm <local_dir> <rm_dest_folder>
# Uploads all PDFs found directly in local_dir to reMarkable.
# Successfully uploaded files are moved to UPLOADED_DIR, preserving the filename.
# ---------------------------------------------------------------------------
upload_to_rm() {
    local local_dir="$1"
    local rm_dest="$2"

    find "${local_dir}" -maxdepth 1 -name "*.pdf" -type f | while read -r pdf_file; do
        local filename
        filename=$(basename "${pdf_file}")
        bashio::log.info "Obsidian → reMarkable: ${filename}"
        if rmapi put "${pdf_file}" "${rm_dest}" 2>&1 | \
                while IFS= read -r line; do bashio::log.debug "${line}"; done; then
            mv "${pdf_file}" "${UPLOADED_DIR}/${filename}"
            bashio::log.info "Uploaded and moved to Uploaded/: ${filename}"
        else
            bashio::log.warning "Upload failed for: ${filename} (will retry next cycle)"
        fi
    done
}

# ---------------------------------------------------------------------------
# Run sync
# ---------------------------------------------------------------------------
bashio::log.info "Syncing reMarkable folder '${RM_FOLDER}' ↔ ${OBS_FOLDER}"

# reMarkable → Obsidian
download_rm_folder "${RM_FOLDER}" "${OBS_FOLDER}"

# Obsidian → reMarkable (if enabled)
if [ "${BIDIRECTIONAL}" = "true" ]; then
    upload_to_rm "${UPLOAD_DIR}" "${RM_FOLDER}"
fi
