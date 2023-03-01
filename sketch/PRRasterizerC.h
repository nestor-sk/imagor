// NOLINTBEGIN
// This is a C file, and clang-tidy is configured for C++

#ifndef PR_RASTERIZER_C_H
#define PR_RASTERIZER_C_H

typedef struct _PRRasterizerContainer PRRasterizerContainer;

typedef struct _PRRasterizerResult {
  char *buffer;
  unsigned long size;
} PRRasterizerResult;

#define EXPORT __attribute__((visibility("default")))

#ifdef __cplusplus
extern "C" {
#endif

extern EXPORT PRRasterizerContainer *PRRasterizerNew(const char *buffer, const unsigned long size);

extern EXPORT PRRasterizerResult *PRRasterizerExportPNG(PRRasterizerContainer *container,
                                                        const float backingScale,
                                                        const int quality);

extern EXPORT void PRRasterizerFree(PRRasterizerContainer *container);

extern EXPORT void PRRasterizerResultFree(PRRasterizerResult *result);

#ifdef __cplusplus
}
#endif

#endif

// NOLINTEND
