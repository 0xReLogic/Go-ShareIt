<div align="center">
  <img src="https://raw.githubusercontent.com/ashleymcnamara/gophers/master/dancing_gopher.gif" width="100"/>
  <h1>Go-ShareIt</h1>
  <p>A simple, secure, and self-hosted file sharing application.</p>
  
  <p>
    <a href="https://github.com/0xReLogic/Go-ShareIt/releases"><img src="https://img.shields.io/github/v/release/0xReLogic/Go-ShareIt?style=for-the-badge"/></a>
    <a href="https://github.com/0xReLogic/Go-ShareIt/blob/main/LICENSE"><img src="https://img.shields.io/github/license/0xReLogic/Go-ShareIt?style=for-the-badge"/></a>
    <a href="https://github.com/0xReLogic/Go-ShareIt/commits/main"><img src="https://img.shields.io/github/last-commit/0xReLogic/Go-ShareIt?style=for-the-badge"/></a>
  </p>
</div>

---

Go-ShareIt is a lightweight, local-first file sharing application built entirely with the Go standard library. It is designed for users who need a quick and secure way to transfer a file with a self-destructing link. Run it on one machine and access it from any other device on the same Wi-Fi network or mobile hotspot to transfer files without using internet data. Once a file is downloaded, it is permanently deleted from the server, ensuring privacy and security.

## Screenshots

| 1. Main Upload Page | 2. Upload Successful | 3. Link Expired |
| :---: | :---: | :---: |
| <img src="https://i.imgur.com/q3t3Nob.png" alt="Main Upload Page" width="100%"> | <img src="https://i.imgur.com/ZPQi7XX.png" alt="Upload Successful" width="100%"> | <img src="https://i.imgur.com/rko7BYF.png" alt="Link Expired" width="100%"> |

## Features

- **Secure One-Time Downloads**: Each generated link is valid for only a single download. 
- **Self-Destructing Files**: After a successful download, the file is immediately and permanently deleted from the server's storage.
- **Large File Support**: Efficiently handles large files of any size through streaming, ensuring low memory usage.
- **Stateless & Always Ready**: After each download, the file and token are instantly destroyed. The server is immediately ready for the next share without needing a restart.
- **Time-Based Expiration**: Links automatically expire after 5 minutes to prevent long-term exposure.
- **Offline Local Transfers**: Share files across devices on the same Wi-Fi or mobile hotspot network without using internet data.
- **Zero External Dependencies**: Built using only the Go standard library, making it lightweight and fast.
- **Cross-Platform**: Comes with a simple build script to generate binaries for Windows, macOS, and Linux.
- **Self-Contained Executable**: All necessary assets, including the web interface, are embedded directly into the executable file. This means you can run it anywhere without needing extra files.
- **Simple Web Interface**: A clean and straightforward interface for uploading files.

## Usage

1.  **Download** the latest release for your operating system from the [Releases](https://github.com/0xReLogic/Go-ShareIt/releases) page.
2.  **Run** the executable file. A terminal window will open, indicating that the server is running.
3.  **Allow Firewall Access**: If your operating system's firewall prompts for permission, make sure to **Allow Access**, especially for **Private Networks**. This is crucial for other devices to connect.
4.  **Start Sharing**:
    -   **On the same computer**: Open your web browser and navigate to `http://localhost:8080`.
    -   **From another device (phone/laptop)**:
        1.  Connect the device to the **same Wi-Fi network or mobile hotspot**.
        2.  Find the local IP address of the computer running Go-ShareIt:
            -   **Windows**: Open Command Prompt (`cmd`) and type `ipconfig`. Look for the `IPv4 Address`.
            -   **macOS/Linux**: Open a terminal and type `ip addr` or `ifconfig`.
        3.  On your other device's browser, enter `http://<YOUR_LOCAL_IP>:8080` (e.g., `http://192.168.1.10:8080`).
5.  Select a file, upload it, and you will receive a unique, secure download link.

## How to Build (For Developers)

To build the project from the source code, you will need to have Go installed. 

1.  Clone the repository:
    ```sh
    git clone https://github.com/0xReLogic/Go-ShareIt.git
    cd Go-ShareIt
    ```
2.  Run the build script provided. This will create a `releases` directory and populate it with binaries for multiple platforms.
    - On Windows:
      ```cmd
      .\build.bat
      ```
    - On macOS/Linux:
      ```sh
      chmod +x build.sh
      ./build.sh
      ```

## For Contributors: Making it Publicly Accessible

The default way to run Share-it is on a local network. If you are developing and want to share a live demo with someone over the internet, you'll need a public URL. For permanent hosting, a service like `fly.io` is recommended. For temporary sharing during development, `ngrok` is an excellent tool.

1.  Download and set up [ngrok](https://ngrok.com/).
2.  Run Go-ShareIt locally (`go run main.go`).
3.  Expose your local server to the internet:
    ```sh
    ngrok http 8080
    ```
4.  `ngrok` will provide you with a public URL (e.g., `https://random-string.ngrok.io`) that you can share.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=0xReLogic/Go-ShareIt&type=Date)](https://www.star-history.com/#0xReLogic/Go-ShareIt&Date)

<div align="center">
  <i>Made with ❤️ by ReLogic</i>
</div>

## Roadmap

Here is the future development plan for Go-ShareIt, based on priority:

### v1.5 - Security Enhancements (Next Priority)
- [ ] **End-to-End Encryption**: Implement client-side encryption so the server can never read the file contents.
- [ ] **Password Protection**: Add an option to protect downloads with a password.
- [ ] **Custom Expiration Settings**: Allow users to set their own expiration time limits.
- [ ] **Token Encryption**: Encrypt the token within the URL to prevent manipulation.

### v2.0 - Optimization & Advanced Features
- [ ] **Multi-File Upload**: Allow users to upload multiple files or an entire folder in a single session.
- [ ] **Automatic Compression**: Automatically compress files before upload to speed up transfers.
- [ ] **CLI Version**: Build a command-line interface (CLI) for power users, aligning with the project's original vision.

---

## How to Contribute

We welcome contributions from everyone to make Go-ShareIt better! If you're interested in helping, here are the areas where we need the most help:

1.  **Implement Roadmap Features**: You can pick up any item from the **v1.5 Roadmap** or **v2.0** above. Features like **password protection** or **token encryption** would be incredibly valuable additions.

2.  **Concurrency Improvements**: We currently use a standard `mutex` for access control. We'd like to implement more sophisticated solutions to handle many concurrent connections:
    -   **For Uploads**: Use a *Worker Pool Pattern* to limit the number of simultaneous uploads.
    -   **For Downloads**: Use a *Channel as a Semaphore* to ensure only one download is active per token.

3.  **Refactoring & Testing**: Help write unit and integration tests to ensure code stability and reliability as new features are added.

Please open a new issue to discuss your ideas or claim an existing one before you start working. Thank you for helping!
