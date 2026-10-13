#if __VERSION__ >= 450
	// VULKAN PATH
	layout(binding = 0) uniform UniformBufferObject {
		mat4 modelview, projection;
	};
	layout(location = 0) in vec2 position;
	layout(location = 1) in vec2 uv;
	layout(location = 0) out vec2 texcoord;
#else
	// OPENGL / GLES PATH
	#define COMPAT_VARYING out
	#define COMPAT_ATTRIBUTE in
	#define COMPAT_TEXTURE texture
	#ifdef GL_ES
		// Mandatory for GLES: High precision for vertex position math
		precision highp float;
		precision highp int;
	#endif

	uniform mat4 modelview, projection;

	COMPAT_ATTRIBUTE vec2 position;
	COMPAT_ATTRIBUTE vec2 uv;
	COMPAT_VARYING vec2 texcoord;
	// The sprite's rectangle in its texture (u0, v0, u1, v1): an atlas page, or the whole texture
	uniform vec4 uvRect;
#endif

void main(void) {
	#if __VERSION__ >= 450
		texcoord = uv;
	#else
		texcoord = uvRect.xy + uv * (uvRect.zw - uvRect.xy);
	#endif
	gl_Position = projection * (modelview * vec4(position, 0.0, 1.0));
	
	#if __VERSION__ >= 450
		// Vulkan's Y-axis is inverted compared to OpenGL
		gl_Position.y = -gl_Position.y;
	#endif
}