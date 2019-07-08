// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include "datadog_agent_rtloader.h"
#include "memory.h"

#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static rtloader_t *rtloader;

char **get_tags(char *id, int highCard)
{
    printf("I'm extending Python tagger.get_tags:\n");
    printf("id: %s\n", id);
    printf("highCard: %d\n", highCard);

    char **data = _malloc(sizeof(*data) * 4);
    data[0] = strdup("tag1");
    data[1] = strdup("tag2");
    data[2] = strdup("tag3");
    data[3] = NULL;
    return data;
}

void submitMetric(char *id, metric_type_t mt, char *name, float val, char **tags,  char *hostname)
{
    printf("I'm extending Python providing aggregator.submit_metric:\n");
    printf("Check id: %s\n", id);
    printf("Metric '%s': %f\n", name, val);
    printf("Tags:\n");
    int i;
    for (i = 0; tags[i]; i++) {
        printf(" %s", tags[i]);
    }
    printf("\n");
    printf("Hostname: %s\n\n", hostname);

    // TODO: cleanup memory
}

char *read_file(const char *path)
{
    FILE *f = fopen(path, "rb");
    fseek(f, 0, SEEK_END);
    long fsize = ftell(f);
    fseek(f, 0, SEEK_SET);

    char *string = _malloc(fsize + 1);
    long read = fread(string, fsize, 1, f);
    if (read < 1) {
        fprintf(stderr, "Error reading file!\n");
    }
    fclose(f);

    string[fsize] = 0;

    return string;
}

int main(int argc, char *argv[])
{
    if (argc < 2) {
        printf("Please run: demo <2|3> [path_to_python_home]. For example:\n\n");
        printf("demo 3 $VIRTUAL_ENV\n");
        return 1;
    }

    // Python home
    char *python_home = NULL;
    if (argc == 3) {
        python_home = argv[2];
    }

    char *init_error = NULL;
    // Embed Python2
    if (strcmp(argv[1], "2") == 0) {
        rtloader = make2(python_home, &init_error);
        if (!rtloader) {
            printf("Unable to init Python2: %s\n", init_error);
            return 1;
        }
    }
    // Embed Python3
    else if (strcmp(argv[1], "3") == 0) {
        rtloader = make3(python_home, &init_error);
        if (!rtloader) {
            printf("Unable to init Python3: %s\n", init_error);
            return 1;
        }
    }
    // Error
    else {
        printf("Unrecognized version: %s, %d\n", argv[1], strncmp(argv[1], "2", strlen(argv[1])));
        return 2;
    }

    // set submitMetric callback
    set_submit_metric_cb(rtloader, submitMetric);
    set_tags_cb(rtloader, get_tags);

    if (!init(rtloader)) {
        printf("Error initializing rtloader: %s\n", get_error(rtloader));
        return 1;
    }

    rtloader_gilstate_t state = ensure_gil(rtloader);

    py_info_t *info = get_py_info(rtloader);
    if (info) {
        printf("Embedding Python version %s\n\tPath: %s\n\n", info->version, info->path);
        rtloader_free(rtloader, info->path);
        rtloader_free(rtloader, info);
    } else {
        printf("Error info is null %s\n", get_error(rtloader));
    }

    // run a script from file
    char *code = read_file("./demo/main.py");
    run_simple_string(rtloader, code);

    // list integration
    char *dd_wheels = get_integration_list(rtloader);
    if (dd_wheels == NULL) {
        printf("error getting integration list: %s\n", get_error(rtloader));
    } else {
        printf("integration: %s\n", dd_wheels);
        rtloader_free(rtloader, dd_wheels);
    }
    return 0;

    // load the Directory check if available
    rtloader_pyobject_t *py_module;
    rtloader_pyobject_t *py_class;

    printf("importing check\n");
    int ok = get_class(rtloader, "datadog_checks.directory", &py_module, &py_class);
    if (!ok) {
        if (has_error(rtloader)) {
            printf("error getting class: %s\n", get_error(rtloader));
        }
        printf("Failed to get_class\n");
        return 1;
    }

    char *version = NULL;
    ok = get_attr_string(rtloader, py_module, "__version__", &version);
    if (!ok) {
        if (has_error(rtloader)) {
            printf("error getting class version: %s\n", get_error(rtloader));
        }
        printf("Failed to get_version\n");
        return 1;
    }

    char *file = NULL;
    ok = get_attr_string(rtloader, py_module, "__file__", &file);
    if (!ok) {
        if (has_error(rtloader)) {
            printf("error getting class file: %s\n", get_error(rtloader));
        }
        printf("Failed to get_file\n");
        return 1;
    }
    if (version != NULL) {
        printf("Successfully imported Directory integration v%s.\n", version);
    } else {
        printf("Successfully imported Directory integration.\n");
    }
    printf("Directory __file__: %s.\n\n", file);
    rtloader_free(rtloader, version);
    rtloader_free(rtloader, file);

    // load the Directory check if available
    rtloader_pyobject_t *check;

    ok = get_check(rtloader, py_class, "", "{directory: \"/\"}", "directoryID", "directory", &check);
    if (!ok) {
        printf("warning: could not get_check with new api: trying with deprecated API\n");
        // clean error
        get_error(rtloader);

        if (!get_check_deprecated(rtloader, py_class, "", "{directory: \"/\"}", "directoryID", "directory", "", &check)) {
            if (has_error(rtloader)) {
                printf("error loading check: %s\n", get_error(rtloader));
            }
            return 1;
        }
    }

    const char *result = run_check(rtloader, check);

    if (result == NULL) {
        printf("Unable to run the check!\n");
        return 1;
    }

    if (strlen(result) == 0) {
        printf("Successfully run the check\n");
    } else {
        printf("Error running the check, output:\n %s\n", result);
    }
    release_gil(rtloader, state);

    printf("Destroying python\n");
    destroy(rtloader);

    return 0;
}
