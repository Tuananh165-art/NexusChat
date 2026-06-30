"use client";

import { useState, useEffect } from "react";
import { FileDown } from "lucide-react";
import type { Message, FilePayload } from "@/lib/constants";
import { fetchDownloadBlobUrl, getFileExtension, isImageExtension } from "@/lib/api";

interface PeerInfo {
  name: string;
  picture: string;
}

interface GalleryItemProps {
  message: Message & { side: "left" | "right" };
  onImageClick: (src: string) => void;
  peerMap: Map<string, PeerInfo>;
}

export default function GalleryItem({ message, onImageClick, peerMap }: GalleryItemProps) {
  const [fileUrl, setFileUrl] = useState<string>("");

  useEffect(() => {
    let objectUrl = "";
    let cancelled = false;

    try {
      const payload: FilePayload = JSON.parse(message.payload);
      setFileUrl("");
      fetchDownloadBlobUrl(payload.object_key).then((url) => {
        if (cancelled) {
          URL.revokeObjectURL(url);
          return;
        }
        objectUrl = url;
        setFileUrl(url);
      });
    } catch {}

    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [message.payload]);

  let ext = "";
  let fileName = "";
  try {
    const payload: FilePayload = JSON.parse(message.payload);
    ext = getFileExtension(payload.object_key);
    fileName = payload.file_name;
  } catch {}

  const isImage = isImageExtension(ext);

  return (
    <div className="relative group rounded-xl overflow-hidden bg-white/5 border border-white/10 aspect-square">
      {isImage && fileUrl ? (
        <img
          src={fileUrl}
          alt={fileName}
          className="w-full h-full object-cover cursor-pointer hover:scale-105 transition-transform"
          onClick={() => onImageClick(fileUrl)}
          loading="lazy"
        />
      ) : (
        <div className="flex flex-col items-center justify-center h-full gap-2 p-2">
          <FileDown className="w-8 h-8 text-text-muted" />
          <span className="text-[10px] text-text-muted text-center truncate w-full">{fileName}</span>
          {fileUrl && (
            <a href={fileUrl} download className="text-[10px] text-accent-cyan hover:underline">Download</a>
          )}
        </div>
      )}
      <div className="absolute bottom-0 inset-x-0 bg-black/60 px-2 py-1 opacity-0 group-hover:opacity-100 transition-opacity">
        <span className="text-[10px] text-text-primary truncate block">{peerMap.get(message.user_id)?.name || "Unknown"}</span>
      </div>
    </div>
  );
}
