#include "gozlib.h"
#include "test_only_utils.h"

#include <stdio.h>
#include <string.h>
#include <zconf.h>
#include <zlib.h>

typedef struct {
  char *input;
  char *output;
  uInt in_len;
  uInt out_len;
  bool fail_write;
} DataStreamer;

DataStreamer make_data_streamer(void) {
  DataStreamer streamer;
  memset(&streamer, 0, sizeof(DataStreamer));
  return streamer;
}

typedef uLong (*CompressStreamFn)(ZStreamState *, int, StreamDataHandler, StreamDataHandler, uInt, uInt, int *);
typedef uLong (*CompressAllFn)(int, void *restrict, uInt, void *restrict, uInt, int *);

uInt in_handler(ZStreamState *state, void *restrict buffer, uInt length) {
  DataStreamer *streamer = state->data_handler;

  if (streamer->in_len == 0) {
    return 0;
  }

  uInt len = length;
  if (streamer->in_len < len) {
    len = streamer->in_len;
  }

  memcpy(buffer, streamer->input, len);

  streamer->in_len = streamer->in_len - len;
  streamer->input = streamer->input + len;

  return len;
}

uInt out_handler(ZStreamState *state, void *restrict buffer, uInt length) {
  DataStreamer *streamer = state->data_handler;

  if (streamer->fail_write) {
    return 0;
  }
  if (streamer->out_len == 0) {
    return 0;
  }

  memcpy(streamer->output, buffer, length);

  streamer->out_len = streamer->out_len - length;
  streamer->output = streamer->output + length;

  return length;
}

void verify_stream_compress(CompressStreamFn compress_fn, const uInt in_len, const uInt out_len, const uInt work_in_len, const uInt work_out_len) {
  // simulate a stream
  char input[in_len];
  char compressed[out_len];

  init_input_buffer_rand(input, in_len);

  ZStreamState zss;
  DataStreamer streamer = make_data_streamer();
  streamer.input = input;
  streamer.output = compressed;
  streamer.in_len = in_len;
  streamer.out_len = out_len;

  zss.data_handler = &streamer;

  int ec = Z_OK;
  uLong compressed_len = compress_fn(&zss, Z_BEST_COMPRESSION, in_handler, out_handler, work_in_len, work_out_len, &ec);
  ASSERT_MSG(ec == Z_OK, "error code should be zero");
  ASSERT_MSG(compressed_len > 0, "compressed length should be greater than zero");
  ASSERT_MSG(compressed_len <= out_len, "output buffer should be large enough for the compressed output");

  // now we uncompress and check it's equal to the original input
  char uncompressed[in_len];
  memset(uncompressed, 0, in_len);

  uLong uncompressed_len = uncompress_buffer_any(compressed, (uInt)compressed_len, uncompressed, in_len, &ec);

  ASSERT_MSG(ec == Z_OK, "fail to uncompress stream data");
  ASSERT_MSG(uncompressed_len == in_len, "uncompressed data is not the same length as the original");
  ASSERT_MSG(memcmp(input, uncompressed, in_len) == 0, "decompressed data is different from input");
}

void test_gzip_compress_stream(void) {
  PRINT_TEST_NAME;
  const uInt in_len = 2237;
  const uInt out_len = in_len; // more than enough space for the output

  const uInt work_in_len = 1024;
  const uInt work_out_len = 211;

  verify_stream_compress(gzip_compress_stream, in_len, out_len, work_in_len, work_out_len);
}

void test_gzip_compress_stream_zero_input(void) {
  PRINT_TEST_NAME;
  const uInt in_len = 0;
  const uInt out_len = 64; // room for the gzip headers and dictionary

  const uInt work_in_len = 64;
  const uInt work_out_len = 64;

  verify_stream_compress(gzip_compress_stream, in_len, out_len, work_in_len, work_out_len);
}

void test_gzip_compress_stream_equal_size_buffers(void) {
  PRINT_TEST_NAME;
  const uInt buffer_size = 1024 * 8;

  verify_stream_compress(gzip_compress_stream, buffer_size, buffer_size, buffer_size, buffer_size);
}

void verify_compress_fail_output(CompressStreamFn compress_fn) {
  const uInt len = 64;
  char input[len];
  char output[len];

  init_input_buffer_rand(input, len);

  ZStreamState zss;
  DataStreamer streamer = make_data_streamer();
  streamer.fail_write = true;
  streamer.input = input;
  streamer.output = output;
  streamer.in_len = len;
  streamer.out_len = len;

  zss.data_handler = &streamer;

  int ec = Z_OK;
  compress_fn(&zss, Z_BEST_COMPRESSION, in_handler, out_handler, len, len, &ec);
  ASSERT_MSG(ec == GOZLIB_STREAM_OUTPUT_WRITE_ERROR, "should have failed to write to output");
}

void test_all_compression_types_fail_stream_output(void) {
  PRINT_TEST_NAME;

  verify_compress_fail_output(gzip_compress_stream);
  verify_compress_fail_output(zlib_compress_stream);
}

void test_zlib_compress_stream(void) {
  PRINT_TEST_NAME;
  const uInt in_len = 762;
  const uInt out_len = in_len; // more than enough space for the output

  const uInt work_in_len = 311;
  const uInt work_out_len = 67;

  verify_stream_compress(zlib_compress_stream, in_len, out_len, work_in_len, work_out_len);
}

void test_zlib_compress_stream_zero_input(void) {
  PRINT_TEST_NAME;
  const uInt in_len = 0;
  const uInt out_len = 20; // room for headers and dictionary

  const uInt work_in_len = 64;
  const uInt work_out_len = 64;

  verify_stream_compress(zlib_compress_stream, in_len, out_len, work_in_len, work_out_len);
}

void test_zlib_compress_stream_equal_size_buffers(void) {
  PRINT_TEST_NAME;
  const uInt buffer_size = 1830;

  verify_stream_compress(zlib_compress_stream, buffer_size, buffer_size, buffer_size, buffer_size);
}

void verify_uncompress_stream(CompressAllFn cfn, InitBufferFn init_buf_fn) {
  const uInt len = 1024 + 51;
  const uInt work_buffer_length = 512;
  const uInt compressed_input_len = len + 100; // make some room for the metadata
  char original_input[len];
  char compressed_input[compressed_input_len];
  char output[len];

  init_buf_fn(original_input, len);

  int ec = Z_OK;
  uLong compressed_len = cfn(Z_BEST_COMPRESSION, original_input, len, compressed_input, compressed_input_len, &ec);
  ASSERT_MSG(ec == Z_OK, "compression error code should be Z_OK");

  ZStreamState zss;
  DataStreamer streamer = make_data_streamer();
  streamer.input = compressed_input;
  streamer.in_len = (uInt)compressed_len;
  streamer.output = output;
  streamer.out_len = len;

  zss.data_handler = &streamer;

  uLong uncompressed_len = uncompress_stream_any(&zss, in_handler, out_handler, work_buffer_length, work_buffer_length, &ec);
  ASSERT_MSG(ec == Z_OK, "uncompress stream should have error code Z_OK");
  ASSERT_MSG(uncompressed_len == len, "uncompressed length should be the same as uncompressed input length");
  ASSERT_MSG(memcmp(original_input, output, uncompressed_len) == 0, "uncompressed stream should be the same as original input");
}

void test_uncompress_gzip_stream(void) {
  PRINT_TEST_NAME;
  verify_uncompress_stream(gzip_compress_buffer, init_input_buffer_rand);
}

void test_uncompress_zlib_stream(void) {
  PRINT_TEST_NAME;
  verify_uncompress_stream(zlib_compress_buffer, init_input_buffer_rand);
}

void test_uncompress_fail_invalid_stream(void) {
  PRINT_TEST_NAME;
  const uInt len = 1024;
  char invalid_input[len];
  char output[len];

  // create an invalid data stream
  init_input_buffer_rand(invalid_input, len);

  ZStreamState zss;
  DataStreamer streamer = make_data_streamer();
  streamer.input = invalid_input;
  streamer.in_len = len;
  streamer.output = output;
  streamer.out_len = len;

  zss.data_handler = &streamer;

  int ec = Z_OK;
  uLong uncompressed_len = uncompress_stream_any(&zss, in_handler, out_handler, len, len, &ec);
  ASSERT_MSG(ec == Z_DATA_ERROR, "uncompressing an invalid stream should fail");
  ASSERT_MSG(uncompressed_len == 0, "uncompressing an invalid stram should return zero bytes");
}

void test_uncompress_fail_stream_output(void) {
  PRINT_TEST_NAME;

  const uInt len = 1024;
  char original_input[len];
  char compressed_input[len];
  char output[len];

  init_input_buffer_rand(original_input, len);

  int ec = Z_OK;
  // zlib or gzip, it shouldn't matter
  uLong compressed_len = zlib_compress_buffer(Z_BEST_COMPRESSION, original_input, len, compressed_input, len, &ec);
  ASSERT_MSG(ec == Z_OK, "compression error code should be Z_OK");

  ZStreamState zss;
  DataStreamer streamer = make_data_streamer();
  streamer.input = compressed_input;
  streamer.in_len = (uInt)compressed_len;

  streamer.output = output;
  streamer.out_len = len;
  streamer.fail_write = true;
  zss.data_handler = &streamer;

  uncompress_stream_any(&zss, in_handler, out_handler, len, len, &ec);
  ASSERT_MSG(ec == GOZLIB_STREAM_OUTPUT_WRITE_ERROR, "fail to write uncompressed stream should result in an error");
}

void test_gzip_compress_stream_compressed_larger_than_input(void) {
  PRINT_TEST_NAME;
  verify_uncompress_stream(gzip_compress_buffer, init_input_buffer_high_entropy);
}

void test_zlib_compress_stream_compressed_larger_than_input(void) {
  PRINT_TEST_NAME;
  verify_uncompress_stream(zlib_compress_buffer, init_input_buffer_high_entropy);
}

int main(void) {
  test_gzip_compress_stream();
  test_gzip_compress_stream_zero_input();
  test_gzip_compress_stream_equal_size_buffers();

  test_zlib_compress_stream();
  test_zlib_compress_stream_zero_input();
  test_zlib_compress_stream_equal_size_buffers();

  test_all_compression_types_fail_stream_output();

  test_uncompress_gzip_stream();
  test_uncompress_zlib_stream();
  test_uncompress_fail_invalid_stream();
  test_uncompress_fail_stream_output();

  test_gzip_compress_stream_compressed_larger_than_input();
  test_zlib_compress_stream_compressed_larger_than_input();

  return 0;
}
