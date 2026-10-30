import * as THREE from "three";
import type {
  Batch,
  MeshData,
  Surface,
  FloatAttribute,
  IndexAttribute,
} from "./types";

const vertexShader = `
  attribute vec3 color;
  attribute vec3 shadeNormal;
  // Keep atlas UV interpolation inside partially covered MSAA edge pixels.
  centroid varying vec2 localUV;
  varying vec3 tint;
  varying vec3 faceNormal;
  void main() {
    localUV = uv;
    tint = color;
    faceNormal = normalize(normalMatrix * shadeNormal);
    gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
  }
`;
const fragmentShader = `
  uniform sampler2D atlasMap;
  uniform vec4 region;
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
  }
`;

function surfaceMaterial(
  surface: Surface,
  atlas: THREE.Texture,
  time: THREE.IUniform<number>,
): THREE.ShaderMaterial {
  const image = atlas.image as { width: number; height: number };
  return new THREE.ShaderMaterial({
    vertexShader,
    fragmentShader,
    uniforms: {
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

// Typed binary views are retained directly. JSON arrays are converted once.
function floats(
  values: FloatAttribute,
  start: number,
  count: number,
): Float32Array {
  return values instanceof Float32Array
    ? values.subarray(start, start + count)
    : new Float32Array(values.slice(start, start + count));
}

function buffers(
  batch: Batch,
  start: number,
  count: number,
  indices: IndexAttribute,
): THREE.BufferGeometry {
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute(
    "position",
    new THREE.BufferAttribute(floats(batch.positions, start * 3, count * 3), 3),
  );
  geometry.setAttribute(
    "normal",
    new THREE.BufferAttribute(floats(batch.normals, start * 3, count * 3), 3),
  );
  geometry.setAttribute(
    "shadeNormal",
    new THREE.BufferAttribute(
      floats(batch.shadeNormals, start * 3, count * 3),
      3,
    ),
  );
  geometry.setAttribute(
    "color",
    new THREE.BufferAttribute(floats(batch.colors, start * 3, count * 3), 3),
  );
  geometry.setAttribute(
    "uv",
    new THREE.BufferAttribute(floats(batch.uvs, start * 2, count * 2), 2),
  );
  geometry.setIndex(
    new THREE.BufferAttribute(
      indices instanceof Uint32Array ? indices : new Uint32Array(indices),
      1,
    ),
  );
  geometry.computeBoundingSphere();
  return geometry;
}

export interface MeshObject {
  object: THREE.Group;
  readonly animated: boolean;
  /** 由宿主在每次绘制前传入时间，单位为秒。 */
  updateTime(seconds: number): void;
  /** Dispose this object's buffers/materials. The shared atlas remains owned by the caller. */
  dispose(): void;
}

/** Draw already baked geometry. This module never receives blockstates/models. */
export function createMeshObject(
  data: MeshData,
  atlas: THREE.Texture,
): MeshObject {
  const object = new THREE.Group();
  const geometries: THREE.BufferGeometry[] = [];
  const materials: THREE.Material[] = [];
  // 所有批次共享时钟，透明面拆分后仍保持同一动画相位。
  const time = { value: 0 };
  const animated = data.batches.some((batch) => batch.surface.frames > 1);
  for (const batch of data.batches) {
    const material = surfaceMaterial(batch.surface, atlas, time);
    materials.push(material);
    const count = batch.positions.length / 3;
    if (batch.surface.mode === "blend") {
      // Three sorts transparent objects, not triangles inside one buffer.
      // Separate static quads make the near/far glass surfaces sortable.
      for (let start = 0; start < count; start += 4) {
        const geometry = buffers(batch, start, 4, [0, 1, 2, 0, 2, 3]);
        geometries.push(geometry);
        object.add(new THREE.Mesh(geometry, material));
      }
    } else {
      const geometry = buffers(batch, 0, count, batch.indices);
      geometries.push(geometry);
      object.add(new THREE.Mesh(geometry, material));
    }
  }
  return {
    object,
    animated,
    updateTime(seconds: number) {
      time.value = seconds;
    },
    dispose() {
      geometries.forEach((g) => g.dispose());
      materials.forEach((m) => m.dispose());
    },
  };
}

export async function loadAtlas(url: string): Promise<THREE.Texture> {
  const texture = await new THREE.TextureLoader().loadAsync(url);
  texture.colorSpace = THREE.SRGBColorSpace;
  texture.flipY = false; // Go's UVs use PNG's top-left origin.
  texture.magFilter = THREE.NearestFilter;
  texture.minFilter = THREE.NearestFilter;
  texture.generateMipmaps = false;
  return texture;
}
