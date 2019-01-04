package com.github.bingoohuang.springboot.uploadserver;


import lombok.Cleanup;
import lombok.SneakyThrows;
import lombok.val;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.multipart.MultipartFile;

import java.io.File;
import java.io.FileOutputStream;
import java.nio.channels.Channels;
import java.util.UUID;

@RestController
public class FileUploadController {
    static {
        new File("/tmp/springbootupload/").mkdirs();
    }

    @PostMapping("/upload") @SneakyThrows
    public String handleFileUpload(@RequestParam("file") MultipartFile file) {
        File tmpFile = new File("/tmp/springbootupload/" + UUID.randomUUID().toString());
        file.transferTo(tmpFile);
        return "ok";
    }

    // Use file channel transfer (zero copy), this is almost half slower than direct file
    // write when benchmarking with 5mb uploading.
    @PostMapping("/upload0") @SneakyThrows
    public String handleFileUpload0(@RequestParam("file") MultipartFile file) {
        File tmpFile = new File("/tmp/springbootupload/" + UUID.randomUUID().toString());

        @Cleanup val source = Channels.newChannel(file.getInputStream());
        @Cleanup val destination = new FileOutputStream(tmpFile).getChannel();
        destination.transferFrom(source, 0, file.getSize());

        file.transferTo(tmpFile);
        return "ok";
    }
}

