#define _GNU_SOURCE
#include <sched.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define OP_TYPE_CREATE "CREATE"
#define OP_TYPE_ENTER "ENTER"
#define OP_TYPE_KEY "OP_TYPE"
#define NS_TYPE_KEY "NS_TYPE"
#define NS_PATH_KEY "NS_PATH"

char msg_arr[1024];

void error(char *msg) {
    printf("error:%s\n", msg);
    exit(0);
}

int get_ns_flag(char *ns_type) {
    if (!strcmp(ns_type, "mnt"))
        return CLONE_NEWNS;
    else if (!strcmp(ns_type, "uts"))
        return CLONE_NEWUTS;
    else if (!strcmp(ns_type, "ipc"))
        return CLONE_NEWIPC;
    return -1;
}

void nsenter(int flag) {
    char *ns_path = getenv(NS_PATH_KEY);
    if (ns_path == NULL)
        error("No ns_path provided");
    if (setns(ns_path, flag))
        error("setns failed");
}

void nscreate(int flag) {
    if (unshare(flag))
        error("unshare failed");
}

void nsexec() {
    char *op_type = getenv(OP_TYPE_KEY);
    if (op_type == NULL)
        error("No op_type provided");
    char *ns_type = getenv(NS_TYPE_KEY);
    if (ns_type == NULL)
        error("No ns_type provided");
    int flag = get_ns_flag(ns_type);
    if (flag == -1) {
        sprintf(msg_arr, "Invalid ns_type %s", ns_type);
        error(msg_arr);
    }
    if (!strcmp(op_type, OP_TYPE_CREATE))
        nscreate(flag);
    else if (!strcmp(op_type, OP_TYPE_ENTER))
        nsenter(flag);
    else {
        sprintf(msg_arr, "Invalid op_type %s", op_type);
        error(msg_arr);
    }
}