#if defined(__TARGET_ARCH_arm64)
    #include "arm64/vmlinux.h"
#elif defined(__TARGET_ARCH_amd64)
    #include "amd64/vmlinux.h"
#elif defined(__TARGET_ARCH_x86)
    #include "amd64/vmlinux.h"
#else
    #error "Unsupported architecture"
#endif
