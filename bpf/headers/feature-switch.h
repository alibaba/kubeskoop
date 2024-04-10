#pragma once

#define FEATURE_SWITCH(probe) \
struct { \
  __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY); \
  __type(key, int); \
  __type(value, u8); \
  __uint(max_entries, 16); \
} insp_##probe##_feature_switch SEC(".maps"); \
static bool inline is_enable(int key){ \
    u8 *val = bpf_map_lookup_elem(&insp_##probe##_feature_switch, &key); \
    return val ? *val : false; \
}



