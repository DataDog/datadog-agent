#!/usr/bin/env python3

import os
import tempfile
import unittest
import subprocess
from pathlib import Path

class TestAPIKeyCheck(unittest.TestCase):
    def setUp(self):
        self.script_path = Path(__file__).parent / "cont-init.d" / "01-check-apikey.sh"
        
    def test_no_api_key_fails(self):
        """Test that script fails when no API key is provided"""
        env = os.environ.copy()
        env.pop('DD_API_KEY', None)
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("site: datadoghq.com\n")
            config_file = f.name
            
        try:
            env['CONFIG_FILE'] = config_file
            result = subprocess.run(['bash', str(self.script_path)], 
                                  env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 1)
            self.assertIn("You must set either", result.stdout)
        finally:
            os.unlink(config_file)
    
    def test_dd_api_key_env_passes(self):
        """Test that script passes with DD_API_KEY env var"""
        env = os.environ.copy()
        env['DD_API_KEY'] = 'test_key'
        
        result = subprocess.run(['bash', str(self.script_path)], 
                              env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0)
    
    def test_config_api_key_passes(self):
        """Test that script passes with api_key in config file"""
        env = os.environ.copy()
        env.pop('DD_API_KEY', None)
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("api_key: test_config_key\nsite: datadoghq.com\n")
            config_file = f.name
            
        try:
            # Modify script to use our test config file
            script_content = self.script_path.read_text()
            test_script_content = script_content.replace(
                'CONFIG_FILE="/etc/datadog-agent/datadog.yaml"',
                f'CONFIG_FILE="{config_file}"'
            )
            
            with tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False) as test_script:
                test_script.write(test_script_content)
                test_script_path = test_script.name
                
            os.chmod(test_script_path, 0o755)
            result = subprocess.run(['bash', test_script_path], 
                                  env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0)
        finally:
            os.unlink(config_file)
            os.unlink(test_script_path)
    
    def test_enc_secret_handle_passes(self):
        """Test that script passes with ENC[...] secret handle"""
        env = os.environ.copy()
        env.pop('DD_API_KEY', None)
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("api_key: ENC[azure_key_vault,secret_name,field_name]\nsite: datadoghq.com\n")
            config_file = f.name
            
        try:
            script_content = self.script_path.read_text()
            test_script_content = script_content.replace(
                'CONFIG_FILE="/etc/datadog-agent/datadog.yaml"',
                f'CONFIG_FILE="{config_file}"'
            )
            
            with tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False) as test_script:
                test_script.write(test_script_content)
                test_script_path = test_script.name
                
            os.chmod(test_script_path, 0o755)
            result = subprocess.run(['bash', test_script_path], 
                                  env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0)
        finally:
            os.unlink(config_file)
            os.unlink(test_script_path)

if __name__ == '__main__':
    unittest.main()
