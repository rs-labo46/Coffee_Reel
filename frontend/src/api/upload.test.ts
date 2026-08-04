import { beforeEach, describe, expect, it, vi } from "vitest";

import { uploadTarget } from "../tests/videoFixtures";
import { uploadVideo, VideoUploadError } from "./upload";

class MockXMLHttpRequest {
  static instances: MockXMLHttpRequest[] = [];

  readonly upload: {
    onprogress: ((event: ProgressEvent<EventTarget>) => void) | null;
  } = {
    onprogress: null,
  };

  status = 0;
  withCredentials = true;
  onload: ((event: ProgressEvent<EventTarget>) => void) | null = null;
  onerror: ((event: ProgressEvent<EventTarget>) => void) | null = null;
  onabort: ((event: ProgressEvent<EventTarget>) => void) | null = null;

  readonly open = vi.fn();
  readonly setRequestHeader = vi.fn();
  readonly send = vi.fn();
  readonly abort = vi.fn(() => {
    this.onabort?.(new ProgressEvent("abort"));
  });

  constructor() {
    MockXMLHttpRequest.instances.push(this);
  }
}

// 最新のXMLHttpRequest Mockを取得
function latestRequest(): MockXMLHttpRequest {
  const request = MockXMLHttpRequest.instances.at(-1);

  if (request === undefined) {
    throw new Error("XMLHttpRequestが生成されていません");
  }

  return request;
}

describe("動画Storage Upload", () => {
  beforeEach(() => {
    MockXMLHttpRequest.instances = [];
    vi.stubGlobal(
      "XMLHttpRequest",
      MockXMLHttpRequest as unknown as typeof XMLHttpRequest,
    );
  });

  it("Presigned URLへContent-Typeだけを設定して動画を直接PUTする", async () => {
    const file = new File([new Uint8Array(10)], "coffee.mp4", {
      type: "video/mp4",
    });
    const onProgress = vi.fn();

    const uploadPromise = uploadVideo({
      file,
      target: uploadTarget(),
      onProgress,
    });
    const request = latestRequest();

    expect(request.open).toHaveBeenCalledWith(
      "PUT",
      "https://storage.example.com/upload",
      true,
    );
    expect(request.withCredentials).toBe(false);
    expect(request.setRequestHeader).toHaveBeenCalledTimes(1);
    expect(request.setRequestHeader).toHaveBeenCalledWith(
      "Content-Type",
      "video/mp4",
    );
    expect(request.send).toHaveBeenCalledWith(file);
    expect(onProgress).toHaveBeenCalledWith({
      loadedBytes: 0,
      totalBytes: 10,
      percentage: 0,
    });

    request.upload.onprogress?.(
      new ProgressEvent("progress", {
        loaded: 5,
        total: 10,
        lengthComputable: true,
      }),
    );
    request.status = 200;
    request.onload?.(new ProgressEvent("load"));

    await expect(uploadPromise).resolves.toBeUndefined();
    expect(onProgress).toHaveBeenCalledWith({
      loadedBytes: 5,
      totalBytes: 10,
      percentage: 50,
    });
    expect(onProgress).toHaveBeenLastCalledWith({
      loadedBytes: 10,
      totalBytes: 10,
      percentage: 100,
    });
  });

  it("Storageが2xx以外を返した場合はStatus付きUpload Errorへ変換する", async () => {
    const uploadPromise = uploadVideo({
      file: new File(["video"], "coffee.mp4", { type: "video/mp4" }),
      target: uploadTarget(),
    });
    const request = latestRequest();

    request.status = 403;
    request.onload?.(new ProgressEvent("load"));

    await expect(uploadPromise).rejects.toMatchObject({
      name: "VideoUploadError",
      code: "upload_failed",
      status: 403,
      message: "動画のアップロードに失敗しました",
    });
  });

  it("Storage接続失敗をnetwork_errorへ変換する", async () => {
    const uploadPromise = uploadVideo({
      file: new File(["video"], "coffee.mp4", { type: "video/mp4" }),
      target: uploadTarget(),
    });
    const request = latestRequest();

    request.onerror?.(new ProgressEvent("error"));

    await expect(uploadPromise).rejects.toMatchObject({
      code: "network_error",
      message: "動画ストレージへ接続できません",
    });
  });

  it("AbortSignalで実行中のUploadを中断する", async () => {
    const controller = new AbortController();
    const uploadPromise = uploadVideo({
      file: new File(["video"], "coffee.mp4", { type: "video/mp4" }),
      target: uploadTarget(),
      signal: controller.signal,
    });
    const request = latestRequest();

    controller.abort();

    expect(request.abort).toHaveBeenCalledTimes(1);
    await expect(uploadPromise).rejects.toMatchObject({
      code: "upload_aborted",
      message: "動画のアップロードが中断されました",
    });
  });

  it("空Fileと不正なUpload Targetを送信前に拒否する", () => {
    expect(() =>
      uploadVideo({
        file: new File([], "empty.mp4", { type: "video/mp4" }),
        target: uploadTarget(),
      }),
    ).toThrowError(
      new VideoUploadError("empty_file", "空の動画はアップロードできません"),
    );

    expect(() =>
      uploadVideo({
        file: new File(["video"], "coffee.mp4", { type: "video/mp4" }),
        target: uploadTarget({
          url: "javascript:alert(1)",
        }),
      }),
    ).toThrowError(
      new VideoUploadError(
        "invalid_upload_target",
        "動画のアップロード先が不正です",
      ),
    );

    expect(MockXMLHttpRequest.instances).toHaveLength(0);
  });
});
