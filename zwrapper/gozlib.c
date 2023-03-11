#include "gozlib.h"
#include "dyn_mem_pool.h"
#include "gozlib_interop.h"

#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <zconf.h>
#include <zlib.h>

#ifdef __GNUC__
#define LIKELY(x) __builtin_expect(!!(x), 1)
#define UNLIKELY(x) __builtin_expect(!!(x), 0)
#else
#define LIKELY(x) (x)
#define UNLIKELY(x) (x)
#endif

#define UNCOMPRESS_ANY_WINDOW_BITS (MAX_WBITS + 32)
#define COMPRESS_GZIP_WINDOW_BITS (MAX_WBITS + 16)

struct MemPool *_zstreamstate_pool = NULL;
struct MemPool *_z_stream_pool = NULL;
struct MemPool *_gozlib_transformer_pool = NULL;

__attribute__((constructor)) void create_mem_pools(void) {
  global_multipool_create();

  _zstreamstate_pool = alloc_mem_pool(sizeof(ZStreamState));
  _z_stream_pool = alloc_mem_pool(sizeof(z_stream));
  _gozlib_transformer_pool = alloc_mem_pool(sizeof(GoZLibTransformer));
}

__attribute__((destructor)) void free_mem_pools(void) {
  global_multipool_free();

  free_mem_pool(_zstreamstate_pool);
  free_mem_pool(_z_stream_pool);
  free_mem_pool(_gozlib_transformer_pool);
}

void *pool_alloc(size_t size) {
  return global_multipool_mem_acquire((uint32_t)size);
}

void pool_free(void *data) {
  pool_mem_return(data);
}

static inline void *zlib_custom_alloc(__attribute__((unused)) void *q, unsigned int nmembers, unsigned int msize) {
  return pool_alloc(nmembers * msize);
}

static inline void zlib_custom_free(__attribute__((unused)) void *q, void *p) {
  pool_free(p);
}

static inline void init_default_zstream(z_streamp zs) {
  zs->zalloc = zlib_custom_alloc;
  zs->zfree = zlib_custom_free;
  zs->opaque = NULL;
}

static inline z_stream make_zstream(void) {
  z_stream zs;
  init_default_zstream(&zs);

  return zs;
}

ZStreamState *pool_acquire_zstream_state(void) {
  return pool_mem_acquire(_zstreamstate_pool);
}

void pool_release_zstream_state(ZStreamState *state) {
  pool_mem_return(state);
}

static inline uLong compress_buffer(int level, void *restrict input, uInt input_len, void *restrict output, uInt output_len, int window_bits, int *error_code) {
  z_stream zs = make_zstream();
  int init_res = deflateInit2(&zs, level, Z_DEFLATED, window_bits, MAX_MEM_LEVEL, Z_DEFAULT_STRATEGY);

  if (init_res != Z_OK) {
    *error_code = init_res;
    return 0;
  }

  zs.next_in = input;
  zs.avail_in = input_len;
  zs.next_out = output;
  zs.avail_out = output_len;

  const int def_code = deflate(&zs, Z_FINISH);

  uLong out_len = zs.total_out;
  if (def_code != Z_STREAM_END) {
    *error_code = def_code;
    // the output buffer should be large enough
    if (def_code == Z_OK) {
      *error_code = Z_MEM_ERROR;
    }
    out_len = 0;
  }

  deflateEnd(&zs);

  return out_len;
}

uLong zlib_compress_buffer(int level, void *restrict input, uInt input_len, void *restrict output, uInt output_len, int *error_code) {
  return compress_buffer(level, input, input_len, output, output_len, MAX_WBITS, error_code);
}

uLong gzip_compress_buffer(int level, void *restrict input, uInt input_len, void *restrict output, uInt output_len, int *restrict error_code) {
  return compress_buffer(level, input, input_len, output, output_len, COMPRESS_GZIP_WINDOW_BITS, error_code);
}

uLong uncompress_buffer_any(void *restrict input, uInt input_len, void *restrict output, uInt output_len, int *restrict error_code) {
  z_stream zs = make_zstream();
  int init_res = inflateInit2(&zs, UNCOMPRESS_ANY_WINDOW_BITS);

  if (init_res != Z_OK) {
    *error_code = init_res;
    return 0;
  }

  zs.next_in = input;
  zs.avail_in = input_len;
  zs.next_out = output;
  zs.avail_out = output_len;

  const int inf_code = inflate(&zs, Z_FINISH);

  uLong out_len = zs.total_out;
  if (UNLIKELY(inf_code != Z_STREAM_END)) {
    *error_code = inf_code;
    // the output buffer should be large enough
    if (inf_code == Z_OK) {
      *error_code = Z_MEM_ERROR;
    }

    // if the input data is not valid, there's not use in hinting the caller about how much we compressed
    if (inf_code != Z_DATA_ERROR) {
      out_len = zs.avail_in;
    }
  }

  inflateEnd(&zs);
  return out_len;
}

int compress_to_outstream(ZStreamState *state, z_streamp zs, int flush, StreamDataHandler output_handler, void *restrict output_buf, uInt output_len) {
  while (true) {
    zs->avail_out = output_len;
    zs->next_out = output_buf;
    int def_code = deflate(zs, flush);

    if (def_code == Z_STREAM_ERROR) {
      return def_code;
    }

    uInt outstream_len = output_len - zs->avail_out;

    if (outstream_len > 0) {
      if (UNLIKELY(output_handler(state, output_buf, outstream_len) == 0)) {
        return GOZLIB_STREAM_OUTPUT_WRITE_ERROR;
      }
    }

    // there's room in the buffer but it's not time to flush it yet
    if (zs->avail_out > 0) {
      return def_code;
    }
  }
}

static inline uLong compress_stream(ZStreamState *state, int level, int window_bits, StreamDataHandler input_handler, StreamDataHandler output_handler, uInt work_input_buffer_cap,
                                    uInt work_output_buffer_cap, int *error_code) {
  z_stream zs = make_zstream();

  int init_code = deflateInit2(&zs, level, Z_DEFLATED, window_bits, MAX_MEM_LEVEL, Z_DEFAULT_STRATEGY);
  if (init_code != Z_OK) {
    *error_code = init_code;
    return 0;
  }

  void *input_buf = pool_alloc((size_t)work_input_buffer_cap);
  void *output_buf = pool_alloc((size_t)work_output_buffer_cap);

  bool do_compress = true;

  while (do_compress) {
    zs.avail_in = input_handler(state, input_buf, work_input_buffer_cap);
    zs.next_in = input_buf;

    do_compress = zs.avail_in > 0;
    int flush = do_compress ? Z_NO_FLUSH : Z_FINISH;
    int comp_code = compress_to_outstream(state, &zs, flush, output_handler, output_buf, work_output_buffer_cap);

    if (comp_code < Z_OK) {
      do_compress = false;
      *error_code = comp_code;
    }
  }

  uLong compressed_len = zs.total_out;
  deflateEnd(&zs);

  pool_free(input_buf);
  pool_free(output_buf);

  return compressed_len;
}

uLong zlib_compress_stream(ZStreamState *state, int level, StreamDataHandler input_handler, StreamDataHandler output_handler, uInt work_input_buffer_cap, uInt work_output_buffer_cap,
                           int *error_code) {
  return compress_stream(state, level, MAX_WBITS, input_handler, output_handler, work_input_buffer_cap, work_output_buffer_cap, error_code);
}

uLong gzip_compress_stream(ZStreamState *state, int level, StreamDataHandler input_handler, StreamDataHandler output_handler, uInt work_input_buffer_cap, uInt work_output_buffer_cap,
                           int *error_code) {
  return compress_stream(state, level, COMPRESS_GZIP_WINDOW_BITS, input_handler, output_handler, work_input_buffer_cap, work_output_buffer_cap, error_code);
}

static inline bool is_inflate_result_fatal(int inf_code) {
  return inf_code == Z_DATA_ERROR || inf_code == Z_STREAM_ERROR || inf_code == Z_MEM_ERROR || inf_code == Z_NEED_DICT;
}

int uncompress_to_outstream_step(ZStreamState *state, z_streamp zs, StreamDataHandler output_handler, void *restrict output_buf, uInt output_len) {
  zs->avail_out = output_len;
  zs->next_out = output_buf;
  int inf_code = inflate(zs, Z_NO_FLUSH);

  if (UNLIKELY(is_inflate_result_fatal(inf_code))) {
    if (inf_code == Z_NEED_DICT) { // consider the need for dictionary an error too
      return Z_DATA_ERROR;
    }
    return inf_code;
  }

  uInt outstream_len = output_len - zs->avail_out;

  if (outstream_len > 0) {
    if (UNLIKELY(output_handler(state, output_buf, outstream_len) == 0)) {
      return GOZLIB_STREAM_OUTPUT_WRITE_ERROR;
    }
  }

  // there's room in the buffer but it's not end of the stream yet
  if (zs->avail_out > 0) {
    return Z_OK;
  }

  return GOZLIB_STREAM_OUTPUT_HAS_MORE_DATA;
}

int uncompress_to_outstream(ZStreamState *state, z_streamp zs, StreamDataHandler output_handler, void *restrict output_buf, uInt output_len) {
  int output_code = GOZLIB_STREAM_OUTPUT_HAS_MORE_DATA;
  while (output_code == GOZLIB_STREAM_OUTPUT_HAS_MORE_DATA) {
    output_code = uncompress_to_outstream_step(state, zs, output_handler, output_buf, output_len);
  }
  return output_code;
}

uLong uncompress_stream_any(ZStreamState *state, StreamDataHandler input_handler, StreamDataHandler output_handler, uInt work_input_buffer_cap, uInt work_output_buffer_cap, int *error_code) {
  z_stream zs = make_zstream();

  int init_code = inflateInit2(&zs, UNCOMPRESS_ANY_WINDOW_BITS);
  if (init_code != Z_OK) {
    *error_code = init_code;
    return 0;
  }

  void *input_buf = pool_alloc((size_t)work_input_buffer_cap);
  void *output_buf = pool_alloc((size_t)work_output_buffer_cap);

  zs.avail_in = input_handler(state, input_buf, work_input_buffer_cap);
  zs.next_in = input_buf;

  while (zs.avail_in > 0) {
    int uncomp_code = uncompress_to_outstream(state, &zs, output_handler, output_buf, work_output_buffer_cap);

    if (uncomp_code < Z_OK) {
      *error_code = uncomp_code;
      break;
    }

    if (uncomp_code == Z_STREAM_END) {
      break;
    }
    zs.avail_in = input_handler(state, input_buf, work_input_buffer_cap);
    zs.next_in = input_buf;
  }

  uLong uncompressed_len = zs.total_out;
  inflateEnd(&zs);

  pool_free(input_buf);
  pool_free(output_buf);

  return uncompressed_len;
}

// transformers

static inline z_streamp pool_alloc_zstream(void) {
  return pool_mem_acquire(_z_stream_pool);
}

static inline GoZLibTransformer *pool_alloc_transformer(uInt work_buffer_cap) {
  // this should come from a pool
  GoZLibTransformer *transformer = pool_mem_acquire(_gozlib_transformer_pool);
  transformer->work_buffer = pool_alloc(work_buffer_cap);
  transformer->work_buffer_cap = work_buffer_cap;
  transformer->state = pool_acquire_zstream_state();
  transformer->zs = pool_alloc_zstream();
  init_default_zstream(transformer->zs);

  return transformer;
}

static inline void pool_release_zstream(z_streamp zs) {
  pool_mem_return(zs);
}

static inline void pool_release_transformer(GoZLibTransformer *transformer) {
  // this will return the transformer to the pool
  pool_release_zstream(transformer->zs);
  pool_release_zstream_state(transformer->state);
  pool_free(transformer->work_buffer);

  pool_mem_return(transformer);
}

GoZLibTransformer *acquire_gzip_compression_transformer(int level, uInt work_buffer_cap, int *error_code) {
  GoZLibTransformer *transformer = pool_alloc_transformer(work_buffer_cap);

  int init_code = deflateInit2(transformer->zs, level, Z_DEFLATED, COMPRESS_GZIP_WINDOW_BITS, MAX_MEM_LEVEL, Z_DEFAULT_STRATEGY);
  if (init_code != Z_OK) {
    *error_code = init_code;
  }

  return transformer;
}

GoZLibTransformer *acquire_zlib_compression_transformer(int level, uInt work_buffer_cap, int *error_code) {
  GoZLibTransformer *transformer = pool_alloc_transformer(work_buffer_cap);

  int init_code = deflateInit2(transformer->zs, level, Z_DEFLATED, MAX_WBITS, MAX_MEM_LEVEL, Z_DEFAULT_STRATEGY);
  if (init_code != Z_OK) {
    *error_code = init_code;
  }

  return transformer;
}

GoZLibTransformer *acquire_uncompression_transformer(uInt work_buffer_cap, int *error_code) {
  GoZLibTransformer *transformer = pool_alloc_transformer(work_buffer_cap);
  int init_res = inflateInit2(transformer->zs, UNCOMPRESS_ANY_WINDOW_BITS);

  if (init_res != Z_OK) {
    *error_code = init_res;
  }

  return transformer;
}

void release_compression_transformer(GoZLibTransformer *transformer) {
  deflateEnd(transformer->zs);
  pool_release_transformer(transformer);
}

void release_uncompression_transformer(GoZLibTransformer *transformer) {
  inflateEnd(transformer->zs);
  pool_release_transformer(transformer);
}

void reset_compression_transformer(GoZLibTransformer *transformer) {
  deflateReset(transformer->zs);
}

void reset_uncompression_transformer(GoZLibTransformer *transformer) {
  inflateReset(transformer->zs);
}
