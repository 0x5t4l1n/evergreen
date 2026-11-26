in vec2 vUV;
uniform sampler2D uTex;
out vec4 oColor;
void main() {
  vec3 c = texture(uTex, vUV).rgb;
  float l = luma(c);
  oColor = vec4(c * sstep(.62, 1.05, l), 1.);
}
