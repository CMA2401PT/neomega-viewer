import type * as THREE from "three";
import { loadAtlas, createEffectRegistry, enchantingTableBookTextureURL, type EffectRegistry } from "@neomega/viewer/render";

export class SceneAssets {
  // 贴图跨存档共享，载入失败后允许重新请求，页面关闭后归还 GPU 资源。
  atlas?: THREE.Texture;
  private book?: THREE.Texture;
  effects: EffectRegistry = new Map();
  private loading?: Promise<void>;

  load(): Promise<void> {
    return this.loading ??= this.loadTextures().catch((error: unknown) => {
      this.loading = undefined;
      throw error;
    });
  }

  private async loadTextures(): Promise<void> {
    this.atlas = await loadAtlas("/resources/atlas.png");
    try {
      this.book = await loadAtlas(enchantingTableBookTextureURL);
      this.effects = createEffectRegistry(this.book);
    } catch {
      // 书本贴图缺失时仍保留附魔台的静态本体。
    }
  }

  dispose(): void {
    this.atlas?.dispose();
    this.book?.dispose();
  }
}
