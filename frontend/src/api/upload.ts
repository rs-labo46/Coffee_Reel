import type { VideoUploadTarget } from "../types/video";

export type VideoUploadProgress = {
  loadedBytes: number;
  totalBytes: number;
  percentage: number;
};

export type UploadVideoOptions = {
  file: File;
  target: VideoUploadTarget;
  signal?: AbortSignal;
  onProgress?: (progress: VideoUploadProgress) => void;
};

export type VideoUploadErrorCode =
  | "invalid_upload_target"
  | "empty_file"
  | "upload_aborted"
  | "network_error"
  | "upload_failed";

export class VideoUploadError extends Error {
  readonly code: VideoUploadErrorCode;
  readonly status: number;

  // Storage Upload失敗を画面側で判定するためのError生成
  constructor(code: VideoUploadErrorCode, message: string, status = 0) {
    super(message);
    this.name = "VideoUploadError";
    this.code = code;
    this.status = status;
  }
}

// Presigned PUT URLへ動画本体を直接送信し、Upload進捗と中断を画面へ通知
export function uploadVideo({
  file,
  target,
  signal,
  onProgress,
}: UploadVideoOptions): Promise<void> {
  validateUploadInput(file, target);

  if (signal?.aborted === true) {
    return Promise.reject(
      new VideoUploadError(
        "upload_aborted",
        "動画のアップロードが中断されました",
      ),
    );
  }

  return new Promise<void>((resolve, reject) => {
    const request = new XMLHttpRequest();

    // AbortSignalからStorage Uploadを中断
    const abortUpload = () => {
      request.abort();
    };

    // 完了後のEventとAbortSignal監視を解除
    const cleanup = () => {
      request.upload.onprogress = null;
      request.onload = null;
      request.onerror = null;
      request.onabort = null;
      signal?.removeEventListener("abort", abortUpload);
    };

    // Storage Upload失敗を共通Errorへ変換
    const rejectUpload = (error: VideoUploadError) => {
      cleanup();
      reject(error);
    };

    request.open(target.method, target.url, true);
    request.withCredentials = false;
    request.setRequestHeader("Content-Type", target.headers["Content-Type"]);

    request.upload.onprogress = (event) => {
      onProgress?.(createProgress(event.loaded, file.size));
    };

    request.onload = () => {
      if (request.status < 200 || request.status >= 300) {
        rejectUpload(
          new VideoUploadError(
            "upload_failed",
            "動画のアップロードに失敗しました",
            request.status,
          ),
        );
        return;
      }

      onProgress?.(createProgress(file.size, file.size));
      cleanup();
      resolve();
    };

    request.onerror = () => {
      rejectUpload(
        new VideoUploadError("network_error", "動画ストレージへ接続できません"),
      );
    };

    request.onabort = () => {
      rejectUpload(
        new VideoUploadError(
          "upload_aborted",
          "動画のアップロードが中断されました",
        ),
      );
    };

    signal?.addEventListener("abort", abortUpload, { once: true });
    onProgress?.(createProgress(0, file.size));
    request.send(file);
  });
}

// FileとBackend発行Upload Targetが直接Uploadへ使用可能か確認
function validateUploadInput(file: File, target: VideoUploadTarget): void {
  if (file.size < 1) {
    throw new VideoUploadError(
      "empty_file",
      "空の動画はアップロードできません",
    );
  }

  const contentType = target.headers["Content-Type"];

  if (
    target.method !== "PUT" ||
    !isValidUploadURL(target.url) ||
    (contentType !== "video/mp4" && contentType !== "video/quicktime")
  ) {
    throw new VideoUploadError(
      "invalid_upload_target",
      "動画のアップロード先が不正です",
    );
  }
}

// Presigned URLが絶対HTTP URLまたはHTTPS URLか確認
function isValidUploadURL(value: string): boolean {
  try {
    const url = new URL(value);

    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

// Upload済みBytesから0〜100の進捗情報を作成
function createProgress(
  loadedBytes: number,
  totalBytes: number,
): VideoUploadProgress {
  const safeLoadedBytes = Math.min(Math.max(loadedBytes, 0), totalBytes);

  return {
    loadedBytes: safeLoadedBytes,
    totalBytes,
    percentage: Math.round((safeLoadedBytes / totalBytes) * 100),
  };
}
