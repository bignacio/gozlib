#ifndef GOZLIB_H
#define GOZLIB_H

#include <stdbool.h>
#include <stdio.h>
#include <zconf.h>
#include <zlib.h>


// custom output codes
#define GOZLIB_CUSTOM_CODE_BASE 1024
#define GOZLIB_STREAM_OUTPUT_WRITE_ERROR (-(GOZLIB_CUSTOM_CODE_BASE + 1))
#define GOZLIB_STREAM_OUTPUT_HAS_MORE_DATA (GOZLIB_CUSTOM_CODE_BASE + 1)


/**
 * @brief Struct to track a zlib stream state for streaming operations
 *
 */
typedef struct  {
    void* data_handler;
} ZStreamState;


/**
 * @brief Compress input into the output buffer using the standard zlib compression
 * If the length of output is too small, zero is returned and eror_code is set to the zlib error code
 *
 * @param level
 * @param input
 * @param input_len
 * @param output
 * @param output_len
 * @param error_code
 * @return uLong
 */
uLong zlib_compress_buffer(int level, void* restrict input, uInt input_len, void* restrict output, uInt output_len, int* error_code);

/**
 * @brief Uncompress input (gzip or zlib) into the output buffer. If the output buffer is too small, error_code is set to the zlib error code
 * and the returned value is the number of bytes remaining to be uncompressed.
 *
 * @param input
 * @param input_len
 * @param output
 * @param output_len
 * @param error_code
 * @return uLong
 */
uLong uncompress_buffer_any(void* restrict input, uInt input_len, void* restrict output, uInt output_len, int* error_code);


/**
 * @brief Compress input into the output buffer using the gzip format.
 * If the length of output is too small, zero is returned and eror_code is set to the zlib error code
 *
 * @param level
 * @param input
 * @param input_len
 * @param output
 * @param output_len
 * @param error_code
 * @return int length of compressed output or 0 on error
 */
uLong gzip_compress_buffer(int level, void* restrict input, uInt input_len, void* restrict output, uInt output_len, int* error_code);

ZStreamState* pool_acquire_zstream_state(void);
void pool_release_zstream_state(ZStreamState* state);


void *pool_alloc(size_t size);
void pool_free(void *data);

/**
 * @brief Handler type for streaming data operations
 *
 */
typedef uInt(*StreamDataHandler)(ZStreamState*, void* restrict, uInt);

/**
 * @brief Compress a stream of data using the standard zlib format
 *
 * @param state
 * @param level
 * @param input_handler
 * @param output_handler
 * @param input_len
 * @param output_len
 * @param error_code
 * @return int
 */
uLong zlib_compress_stream(ZStreamState* state, int level, StreamDataHandler input_handler, StreamDataHandler output_handler, uInt work_input_buffer_cap, uInt work_output_buffer_cap, int* error_code);


/**
 * @brief Compress a stream of data using the gzip format
 *
 * @param state
 * @param level
 * @param input_handler
 * @param output_handler
 * @param input_len
 * @param output_len
 * @param error_code
 * @return int
 */
uLong gzip_compress_stream(ZStreamState* state, int level, StreamDataHandler input_handler, StreamDataHandler output_handler, uInt work_input_buffer_cap, uInt work_output_buffer_cap, int* error_code);

/**
 * @brief Uncompress a gzip or zlib compressed stream
 *
 * @param state
 * @param input_handler
 * @param output_handler
 * @param input_len
 * @param output_len
 * @param error_code
 * @return uLong
 */
uLong uncompress_stream_any(ZStreamState* state, StreamDataHandler input_handler, StreamDataHandler output_handler, uInt work_input_buffer_cap, uInt work_output_buffer_cap, int* error_code);


/**
 * @brief Performs one compression step writing to the given output handler
 *
 * @param state
 * @param zs
 * @param flush
 * @param output_handler
 * @param output_buf
 * @param output_len
 * @return int
 */
int compress_to_outstream(ZStreamState *state, z_streamp zs, int flush, StreamDataHandler output_handler, void *restrict output_buf, uInt output_len);

/**
 * @brief Performs one uncompression step writing directly to the output buffer and making it available to the given output handler
 *
 * @param state
 * @param zs
 * @param output_handler
 * @param output_buf
 * @param output_len
 * @param work_buffer_len
 * @return int
 */
int uncompress_to_outstream_step(ZStreamState *state, z_streamp zs, StreamDataHandler output_handler, void *restrict output_buf, uInt output_len);

/**
 * @brief Generic struct for IO Go io.Reader/Writer transformations
 *
 */
typedef struct {
    z_streamp zs;
    ZStreamState* state;
    void* work_buffer;
    uInt work_buffer_cap;
} GoZLibTransformer;

/**
 * @brief Acquires a gzip compression transformer
 *
 * @param level
 * @param output_buffer_cap
 * @param error_code
 * @return GoZLibTransformer
 */
GoZLibTransformer* acquire_gzip_compression_transformer(int level, uInt work_buffer_cap, int* error_code);


/**
 * @brief Releases a gzip or zlib compression transformer
 *
 * @param transformer
 */
void release_compression_transformer(GoZLibTransformer* transformer);

/**
 * @brief Resets a compressor transformer so that it can be reused
 *
 * @param transformer
 */
void reset_compression_transformer(GoZLibTransformer* transformer);

/**
 * @brief Resets an uncompressor transformer so that it can be reused
 *
 * @param transformer
 */
void reset_uncompression_transformer(GoZLibTransformer* transformer);

/**
 * @brief Acquires a zlib compression transformer
 *
 * @param level
 * @param output_buffer_cap
 * @param error_code
 * @return GoZLibTransformer
 */
GoZLibTransformer* acquire_zlib_compression_transformer(int level, uInt work_buffer_cap, int* error_code);

/**
 * @brief Acquires an uncompression transformer
 *
 * @param level
 * @param output_buffer_cap
 * @param error_code
 * @return GoZLibTransformer
 */
GoZLibTransformer* acquire_uncompression_transformer(uInt work_buffer_cap, int* error_code);

/**
 * @brief Releases an uncompression transformer
 *
 * @param transformer
 */
void release_uncompression_transformer(GoZLibTransformer* transformer);


#endif // GOZLIB_H
