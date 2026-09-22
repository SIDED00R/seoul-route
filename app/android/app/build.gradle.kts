plugins {
    id("com.android.application")
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "kr.seoulroute.seoul_route"
    compileSdk = flutter.compileSdkVersion
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        applicationId = "kr.seoulroute.seoul_route"
        minSdk = flutter.minSdkVersion
        targetSdk = flutter.targetSdkVersion
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    // 개발계·운영 분리. dev 는 패키지 뒤에 .dev 가 붙어 운영 앱과 한 폰에 같이 설치되고, 이름에 Dev 가 붙는다.
    // Google 로그인은 flavor 마다 Google Cloud 콘솔 Android 클라이언트(패키지명 + 서명 SHA-1)가 따로 필요하다.
    // flutter run/build 에 --flavor dev 또는 --flavor prod 를 반드시 준다.
    buildFeatures {
        resValues = true // AGP 9 는 기본 꺼짐. flavor 별 app_name 을 만든다
    }
    flavorDimensions += "env"
    productFlavors {
        create("dev") {
            dimension = "env"
            applicationIdSuffix = ".dev"
            resValue("string", "app_name", "서울 길찾기 Dev")
        }
        create("prod") {
            dimension = "env"
            resValue("string", "app_name", "서울 길찾기")
        }
    }

    buildTypes {
        release {
            // 로컬 release 실행용이며 배포 전 별도 서명이 필요하다.
            signingConfig = signingConfigs.getByName("debug")
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget = org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17
    }
}

flutter {
    source = "../.."
}
