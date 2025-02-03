#ifndef DI_MACROS_H
#define DI_MACROS_H

#define MAX_STRING_SIZE {{ .InstrumentationInfo.InstrumentationOptions.StringMaxSize }}
#define PARAM_BUFFER_SIZE {{ .InstrumentationInfo.InstrumentationOptions.ArgumentsMaxSize }}
#define STACK_DEPTH_LIMIT 10
#define MAX_SLICE_LENGTH {{ .InstrumentationInfo.InstrumentationOptions.SliceMaxLength }}

#endif
