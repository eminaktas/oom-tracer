/* SPDX-License-Identifier: (LGPL-2.1 OR BSD-2-Clause) */
#ifndef __VMLINUX_H__
#define __VMLINUX_H__

/* disable default preserve_access_index attribute */
#define BPF_NO_PRESERVE_ACCESS_INDEX

#if defined(__TARGET_ARCH_x86)
#include "vmlinux_generated_x86_64.h"
#elif defined(__TARGET_ARCH_arm64)
#include "vmlinux_generated_aarch64.h"
#endif

#endif /* __VMLINUX_H__ */
