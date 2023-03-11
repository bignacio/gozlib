#ifndef GOZLIB_INTEROP_H
#define GOZLIB_INTEROP_H

#ifdef GOZLIB_GO_INTEROP

#include <string.h>
#include <zlib.h>
#include "gozlib.h"

extern uInt GoStreamDataInputHandler(void *token, void* restrict buffer, uInt buffer_length);
extern uInt GoStreamDataOutputHandler(void *token, void* restrict buffer, uInt buffer_length);

static inline uInt go_stream_data_input_handler(ZStreamState *state, void* restrict buffer, uInt buffer_length) {
    return GoStreamDataInputHandler(state->data_handler, buffer, buffer_length);
}

static inline uInt go_stream_data_output_handler(ZStreamState *state, void* restrict buffer, uInt buffer_length) {
    return GoStreamDataOutputHandler(state->data_handler, buffer, buffer_length);
}

uLong go_gzip_compress_stream(ZStreamState *state, int level, uInt input_cap, uInt output_cap, int *error_code) {
    return gzip_compress_stream(state, level, go_stream_data_input_handler, go_stream_data_output_handler, input_cap, output_cap, error_code);
}

uLong go_uncompress_stream(ZStreamState* state, uInt input_cap, uInt output_cap, int *error_code) {
    return uncompress_stream_any(state, go_stream_data_input_handler, go_stream_data_output_handler, input_cap, output_cap, error_code);
}

int go_transformer_compress_to_outstream(GoZLibTransformer* transformer, void* restrict buffer, uInt buffer_length) {
    transformer->zs->avail_in = buffer_length;
    transformer->zs->next_in = buffer;
    int flush = buffer_length > 0 ? Z_NO_FLUSH : Z_FINISH;
    return compress_to_outstream(transformer->state, transformer->zs, flush, go_stream_data_output_handler, transformer->work_buffer, transformer->work_buffer_cap);
}


void go_assign_uncompress_input(GoZLibTransformer* transformer, uInt work_buffer_len) {
    // input data is in the work buffer but we don't know how much of it can be used
    transformer->zs->avail_in = work_buffer_len;
    transformer->zs->next_in = transformer->work_buffer;
}

int go_uncompress_to_outstream_step(GoZLibTransformer* transformer, void *restrict output_buf, uInt output_len) {
    return uncompress_to_outstream_step(transformer->state, transformer->zs, go_stream_data_output_handler, output_buf, output_len);
}

#endif // GOZLIB_GO_INTEROP


#endif //GOZLIB_EXTERN_H
