import * as THREE from "three";

export interface OcclusionTarget {
  bounds: THREE.Box3;
}
interface Result {
  hidden: boolean;
  query?: WebGLQuery;
}

const QUERY_BUDGET = 8;

/** Query enclosing boxes against the current opaque depth buffer, without writes.
 * Only fully on-screen boxes are tested: a negative result then describes the
 * whole target, and remains useful when rotating around the same eye position.
 * Translation or changes to resident geometry invalidate all results. */
export class OcclusionQueries {
  readonly stats = { queries: 0, pending: 0 };
  private readonly gl: WebGL2RenderingContext;
  private readonly renderer: THREE.WebGLRenderer;
  private readonly results = new Map<OcclusionTarget, Result>();
  private readonly eye = new THREE.Vector3(Infinity, Infinity, Infinity);
  private readonly currentEye = new THREE.Vector3();
  private readonly projection = new THREE.Matrix4();
  private readonly size = new THREE.Vector2();
  private readonly currentSize = new THREE.Vector2();
  private readonly orientation = new THREE.Quaternion();
  private readonly currentOrientation = new THREE.Quaternion();
  private settling = 0;
  private readonly box = new THREE.Box3();
  private readonly queryScene = new THREE.Scene();
  private readonly geometry = new THREE.BoxGeometry(1, 1, 1);
  private readonly material = new THREE.MeshBasicMaterial({
    colorWrite: false,
    depthWrite: false,
    depthFunc: THREE.LessEqualDepth,
    side: THREE.DoubleSide,
  });
  private readonly proxies: THREE.Mesh[] = [];
  private stationary = false;

  constructor(renderer: THREE.WebGLRenderer) {
    this.renderer = renderer;
    this.gl = renderer.getContext() as WebGL2RenderingContext;
    for (let i = 0; i < QUERY_BUDGET; i++) {
      const proxy = new THREE.Mesh(this.geometry, this.material);
      proxy.frustumCulled = false;
      this.proxies.push(proxy);
      this.queryScene.add(proxy);
    }
  }

  begin(camera: THREE.PerspectiveCamera): void {
    camera.getWorldPosition(this.currentEye);
    camera.getWorldQuaternion(this.currentOrientation);
    this.renderer.getDrawingBufferSize(this.currentSize);
    // Rotation changes the subpixel sampling grid even with a fixed eye. Keep
    // temporal results while turning, then recheck once the view settles.
    if (!this.orientation.equals(this.currentOrientation)) this.settling = 2;
    else if (this.settling > 0 && --this.settling === 0) this.invalidate();
    this.orientation.copy(this.currentOrientation);
    this.stationary = this.eye.equals(this.currentEye);
    if (!this.stationary || !this.projection.equals(camera.projectionMatrix)
      || !this.size.equals(this.currentSize) || this.gl.isContextLost()) {
      this.invalidate();
      this.eye.copy(this.currentEye);
      this.projection.copy(camera.projectionMatrix);
      this.size.copy(this.currentSize);
    }
    this.stats.pending = 0;
    for (const result of this.results.values()) {
      if (!result.query) continue;
      // Never wait for the GPU, and never reuse a query while it is in flight.
      if (this.gl.getQueryParameter(result.query, this.gl.QUERY_RESULT_AVAILABLE)) {
        result.hidden = !this.gl.getQueryParameter(result.query, this.gl.QUERY_RESULT);
        this.gl.deleteQuery(result.query);
        result.query = undefined;
      } else {
        this.stats.pending++;
      }
    }
  }

  hidden(key: OcclusionTarget): boolean {
    return this.results.get(key)?.hidden ?? false;
  }

  invalidate(): void {
    for (const result of this.results.values()) {
      if (result.query) this.gl.deleteQuery(result.query);
    }
    this.results.clear();
    this.stats.pending = 0;
  }

  prepare(
    targets: Iterable<OcclusionTarget>,
    frustum: THREE.Frustum,
  ): boolean {
    this.stats.queries = 0;
    if (!this.stationary || this.gl.isContextLost()) return false;
    for (const proxy of this.proxies) proxy.visible = false;
    for (const target of targets) {
      if (this.results.has(target)) continue;
      // Padding avoids rejecting a target against its own coplanar surfaces.
      this.box.copy(target.bounds).expandByScalar(0.05);
      if (!fullyInside(frustum, this.box)) continue;
      const query = this.gl.createQuery();
      if (!query) continue;
      this.results.set(target, { hidden: false, query });
      const proxy = this.proxies[this.stats.queries++];
      this.box.getCenter(proxy.position);
      this.box.getSize(proxy.scale);
      proxy.visible = true;
      proxy.onBeforeRender = () => this.gl.beginQuery(this.gl.ANY_SAMPLES_PASSED_CONSERVATIVE, query);
      proxy.onAfterRender = () => this.gl.endQuery(this.gl.ANY_SAMPLES_PASSED_CONSERVATIVE);
      if (this.stats.queries === QUERY_BUDGET) break;
    }
    return this.stats.queries !== 0;
  }

  issue(camera: THREE.PerspectiveCamera): void {
    this.renderer.render(this.queryScene, camera);
  }

  dispose(): void {
    this.invalidate();
    this.geometry.dispose();
    this.material.dispose();
  }
}

/** Reject boxes touching the near plane or screen edges as well as offscreen ones. */
export function fullyInside(frustum: THREE.Frustum, box: THREE.Box3): boolean {
  for (const { normal: n, constant } of frustum.planes) {
    if (constant
      + n.x * (n.x >= 0 ? box.min.x : box.max.x)
      + n.y * (n.y >= 0 ? box.min.y : box.max.y)
      + n.z * (n.z >= 0 ? box.min.z : box.max.z) <= 0) return false;
  }
  return true;
}
