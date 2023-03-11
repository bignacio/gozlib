#include "gozlib.h"
#include "test_only_utils.h"
#include <assert.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <zconf.h>
#include <zlib.h>

typedef uLong (*CompressFn)(int, void *, uInt, void *, uInt, int *);
typedef uLong (*UncompressFn)(void *, uInt, void *, uInt, int *);

void verify_direct_compress(CompressFn cfn, int level) {
  const uInt length = 1024;
  const uInt output_length = length + 100; // make some room for meta information
  char input[length];
  char output[output_length];

  init_input_buffer_rand(input, length);

  int ec = 0;
  uLong compressed_len = cfn(level, input, length, output, length + 100, &ec);
  ASSERT_MSG(ec == Z_OK, "compressing error code should be Z_OK");
  ASSERT_MSG(compressed_len < output_length, "The output buffer length should be large enough for the compressed output");
}

void verify_direct_compress_uncompress_rand_input(CompressFn cfn, int level, InitBufferFn buf_init_fn) {
  const uInt length = 1024;
  const uInt output_length = length + 100;
  char input[length];
  char compressed[output_length];

  buf_init_fn(input, length);

  int ec = 0;
  uLong compressed_len = cfn(level, input, length, compressed, output_length, &ec);
  ASSERT_MSG(ec == Z_OK, "compressing should return error code Z_OK");
  ASSERT_MSG(compressed_len <= output_length, "compression output buffer should be large enough");

  char uncompressed[length];
  memset(uncompressed, 0, length);
  uLong uncompressed_len = uncompress_buffer_any(compressed, (uInt)compressed_len, uncompressed, length, &ec);
  ASSERT_MSG(ec == Z_OK, "error code should be Z_OK");
  ASSERT_MSG(uncompressed_len == length, "uncompressed length should be equal to input length");

  ASSERT_MSG(memcmp(input, uncompressed, length) == 0, "uncompressed data should be equal to input");
}

void verify_direct_compress_uncompress(CompressFn cfn, int level) {
  verify_direct_compress_uncompress_rand_input(cfn, level, init_input_buffer_rand);
}

void test_gzip_compress(void) {
  PRINT_TEST_NAME;
  verify_direct_compress(gzip_compress_buffer, Z_BEST_SPEED);
}

void test_zlib_compress(void) {
  PRINT_TEST_NAME;
  verify_direct_compress(zlib_compress_buffer, Z_BEST_COMPRESSION);
}

void test_gzip_transform_compress_uncompress(void) {
  PRINT_TEST_NAME;
  verify_direct_compress_uncompress(gzip_compress_buffer, Z_BEST_COMPRESSION);
}

void test_zlib_transform_compress_uncompress(void) {
  PRINT_TEST_NAME;
  verify_direct_compress_uncompress(zlib_compress_buffer, Z_BEST_SPEED);
}

void test_fail_gzip_zlib_compress_small_buffer(void) {
  PRINT_TEST_NAME;

  const uInt length = 1024;
  const uInt output_length = 40;
  char input[length];
  char output[output_length];

  init_input_buffer_rand(input, length);

  int ec = 0;
  uLong compressed_len = zlib_compress_buffer(Z_BEST_COMPRESSION, input, length, output, output_length, &ec);
  assert(ec == Z_MEM_ERROR);
  assert(compressed_len == 0);

  ec = 0;
  compressed_len = gzip_compress_buffer(Z_BEST_COMPRESSION, input, length, output, output_length, &ec);
  assert(ec == Z_MEM_ERROR);
  assert(compressed_len == 0);
}

void test_fail_uncompress_small_buffer(void) {
  PRINT_TEST_NAME;

  const uInt length = 1024;
  const uInt output_length = length + 100;
  char input[length];
  char compressed[output_length];

  init_input_buffer_rand(input, length);

  int ec = 0;
  uLong compressed_len = gzip_compress_buffer(Z_BEST_SPEED, input, length, compressed, output_length, &ec);
  assert(ec == Z_OK);

  const uInt uncompressed_output_length = 100;
  char uncompressed[uncompressed_output_length];
  uLong uncompressed_len = uncompress_buffer_any(compressed, (uInt)compressed_len, uncompressed, uncompressed_output_length, &ec);
  ASSERT_MSG(ec == Z_BUF_ERROR, "compressing with a small output buffer should return an error");
  ASSERT_MSG(uncompressed_len > 1, "number of bytes still uncompressed should be greater than one");
}

void test_gzip_compressed_length_larget_than_input(void) {
  PRINT_TEST_NAME;
  verify_direct_compress_uncompress_rand_input(gzip_compress_buffer, Z_BEST_COMPRESSION, init_input_buffer_high_entropy);
}

void test_zlib_compressed_length_larget_than_input(void) {
  PRINT_TEST_NAME;
  verify_direct_compress_uncompress_rand_input(zlib_compress_buffer, Z_BEST_COMPRESSION, init_input_buffer_high_entropy);
}

void test_fail_transform_uncompress_invalid_input(void) {
  PRINT_TEST_NAME;
  const uInt input_length = 1126;
  const uInt output_length = input_length + 100;
  char invalid_input[input_length];
  char output[output_length];

  // create an invalid input buffer, i.e not zlib nor gzip
  init_input_buffer_rand(invalid_input, input_length);

  int ec = 0;
  uLong uncompressed_len = uncompress_buffer_any(invalid_input, input_length, output, output_length, &ec);
  ASSERT_MSG(ec == Z_DATA_ERROR, "uncompressing invalid data should fail");
  ASSERT_MSG(uncompressed_len == 0, "uncompressing invalid data should return zero");
}

int main(void) {
  test_zlib_compress();
  test_gzip_compress();
  test_gzip_transform_compress_uncompress();
  test_zlib_transform_compress_uncompress();

  test_fail_gzip_zlib_compress_small_buffer();
  test_fail_uncompress_small_buffer();

  test_gzip_compressed_length_larget_than_input();
  test_zlib_compressed_length_larget_than_input();

  test_fail_transform_uncompress_invalid_input();

  return 0;
}
