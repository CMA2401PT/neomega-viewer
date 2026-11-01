import * as THREE from "three";
import type { Surface } from "./types.ts";

const vertexShader = `
  #include <fog_pars_vertex>
  attribute vec3 color;
  attribute vec3 shadeNormal;
  // Keep atlas UV interpolation inside partially covered MSAA edge pixels.
  centroid varying vec2 localUV;
  varying vec3 tint;
  varying vec3 faceNormal;
  #ifdef VERTEX_ATLAS_REGION
    attribute vec4 atlasRegion;
    flat varying vec4 surfaceRegion;
  #endif
  void main() {
    #ifdef VERTEX_ATLAS_REGION
      surfaceRegion = atlasRegion;
    #endif
    localUV = uv;
    tint = color;
    // Regional batches bake their translation into positions once on the CPU.
    // No per-vertex matrix-texture lookup is needed for static world geometry.
    faceNormal = normalize(normalMatrix * shadeNormal);
    vec4 mvPosition = modelViewMatrix * vec4(position, 1.0);
    gl_Position = projectionMatrix * mvPosition;
    #include <fog_vertex>
  }
`;
const fragmentShader = `
  #include <fog_pars_fragment>
  uniform sampler2D atlasMap;
  #ifdef VERTEX_ATLAS_REGION
    flat varying vec4 surfaceRegion;
    #define region surfaceRegion
  #else
    uniform vec4 region;
  #endif
  uniform vec2 atlasSize;
  uniform float frames;
  uniform float frameTime;
  uniform float time;
  uniform float opacity;
  uniform float cutoff;
  uniform float unlit;
  centroid varying vec2 localUV;
  varying vec3 tint;
  varying vec3 faceNormal;
  float spriteUV(float value) {
    // Interpolation can put a face endpoint just outside [0,1]. Wrapping that
    // rounding error would draw the bottom texel along a transparent top edge.
    if (value >= -0.00001 && value <= 1.00001) return clamp(value, 0.0, 1.0);
    return fract(value);
  }
  void main() {
    vec4 frameRegion = region;
    if (frames > 1.0) {
      float frame = mod(floor(time / (frameTime / 20.0)), frames);
      frameRegion.w = region.w / frames;
      frameRegion.y += frame * frameRegion.w;
    }
    // 在当前帧内平铺，半像素内缩避免采到相邻帧。
    vec2 inset = vec2(0.5) / (frameRegion.zw * atlasSize);
    vec2 uv = clamp(vec2(spriteUV(localUV.x), spriteUV(localUV.y)), inset, vec2(1.0) - inset);
    vec4 texel = texture2D(atlasMap, frameRegion.xy + uv * frameRegion.zw);
    float alpha = texel.a * opacity;
    if (alpha < cutoff) discard;
    vec3 n = normalize(faceNormal);
    if (!gl_FrontFacing) n = -n;
    vec3 lightDirection = normalize(mat3(viewMatrix) * vec3(0.35,0.85,0.5));
    float light = mix(0.48 + 0.52 * max(dot(n, lightDirection), 0.0), 1.0, unlit);
    gl_FragColor = vec4(texel.rgb * tint * light, alpha);
    #include <colorspace_fragment>
    #include <fog_fragment>
  }
`;

export function surfaceMaterial(
  surface: Surface,
  atlas: THREE.Texture,
  time: THREE.IUniform<number>,
  vertexAtlasRegion = false,
): THREE.ShaderMaterial {
  const image = atlas.image as { width: number; height: number };
  return new THREE.ShaderMaterial({
    vertexShader,
    fragmentShader,
    defines: vertexAtlasRegion ? { VERTEX_ATLAS_REGION: 1 } : {},
    fog: true,
    uniforms: {
      ...THREE.UniformsUtils.clone(THREE.UniformsLib.fog),
      atlasMap: { value: atlas },
      region: { value: new THREE.Vector4(...surface.region) },
      atlasSize: { value: new THREE.Vector2(image.width, image.height) },
      frames: { value: surface.frames },
      frameTime: { value: surface.frameTime },
      time,
      opacity: { value: surface.opacity },
      cutoff: {
        value:
          surface.mode === "opaque"
            ? 0
            : surface.mode === "cutout"
              ? 0.1
              : 0.001,
      },
      unlit: { value: surface.unlit ? 1 : 0 },
    },
    side: surface.doubleSided ? THREE.DoubleSide : THREE.FrontSide,
    transparent: surface.mode === "blend",
    depthWrite: surface.mode !== "blend",
    forceSinglePass: true,
    toneMapped: false,
  });
}

/** Shader settings that must agree within an opaque draw. Atlas regions are attributes. */
export function surfaceStateKey(surface: Surface): string {
  return JSON.stringify([
    surface.mode,
    surface.doubleSided,
    surface.unlit,
    surface.opacity,
    surface.frames,
    surface.frameTime,
  ]);
}

/** World-owned materials and animation clock; dispose after all world draws are removed. */
export class SurfaceMaterials {
  readonly time = { value: 0 };
  private readonly cache = new Map<string, THREE.ShaderMaterial>();
  private wireframe = false;

  private readonly atlas: THREE.Texture;

  constructor(atlas: THREE.Texture) {
    this.atlas = atlas;
  }

  get(surface: Surface, vertexAtlasRegion = false): THREE.ShaderMaterial {
    const key = surfaceStateKey(surface) + (vertexAtlasRegion
      ? ":vertex"
      : JSON.stringify(surface.region));
    let material = this.cache.get(key);
    if (!material) {
      material = surfaceMaterial(surface, this.atlas, this.time, vertexAtlasRegion);
      material.wireframe = this.wireframe;
      this.cache.set(key, material);
    }
    return material;
  }

  setWireframe(value: boolean): void {
    this.wireframe = value;
    for (const material of this.cache.values()) material.wireframe = value;
  }

  dispose(): void {
    for (const material of this.cache.values()) material.dispose();
    this.cache.clear();
  }
}
