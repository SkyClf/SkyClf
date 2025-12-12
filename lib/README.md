# ONNX Runtime Library

This folder should contain the ONNX Runtime shared library.

## Windows

Download `onnxruntime.dll` and place it here:

1. Go to: https://github.com/microsoft/onnxruntime/releases
2. Download `onnxruntime-win-x64-X.XX.X.zip`
3. Extract and copy `lib/onnxruntime.dll` to this folder

Or run in PowerShell:
```powershell
$v = "1.16.3"
Invoke-WebRequest "https://github.com/microsoft/onnxruntime/releases/download/v$v/onnxruntime-win-x64-$v.zip" -OutFile ort.zip
Expand-Archive ort.zip -DestinationPath .
Copy-Item "onnxruntime-win-x64-$v\lib\onnxruntime.dll" .
Remove-Item ort.zip, "onnxruntime-win-x64-$v" -Recurse
```

## Linux

Download `libonnxruntime.so`:

```bash
v=1.16.3
wget https://github.com/microsoft/onnxruntime/releases/download/v$v/onnxruntime-linux-x64-$v.tgz
tar xzf onnxruntime-linux-x64-$v.tgz
cp onnxruntime-linux-x64-$v/lib/libonnxruntime.so* .
rm -rf onnxruntime-linux-x64-$v*
```

## Configuration

Set in `.env`:
```
SKYCLF_ORT_LIB=./lib/onnxruntime.dll   # Windows
SKYCLF_ORT_LIB=./lib/libonnxruntime.so # Linux
```
